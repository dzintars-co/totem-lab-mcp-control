// Package serial owns one port, serializes commands and confirms them only by matching ACKs.
package serial

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	serialport "go.bug.st/serial"

	"totem-mcp/internal/panel"
	"totem-mcp/internal/protocol"
)

type Config struct {
	Port           string
	Baud           int
	BootDelay      time.Duration
	ReconnectDelay time.Duration
	CommandTimeout time.Duration
}

type response struct {
	reading panel.Reading
	err     error
}
type request struct {
	ctx         context.Context
	kind, value string
	reply       chan response
}
type pending struct {
	id      uint32
	request *request
}

type Controller struct {
	config   Config
	store    *panel.Store
	logger   *slog.Logger
	commands chan request
	nextID   uint32 // Owned exclusively by Run.
}

func New(config Config, store *panel.Store, logger *slog.Logger) *Controller {
	return &Controller{config: config, store: store, logger: logger, commands: make(chan request)}
}

func (c *Controller) Execute(ctx context.Context, kind, value string) (panel.Reading, error) {
	var zero panel.Reading
	if kind == "RGB" {
		color, err := protocol.Color(value)
		if err != nil {
			return zero, err
		}
		value = color
	}
	if _, err := protocol.Command(kind, 1, value); err != nil {
		return zero, err
	}
	if !c.store.View(nil, "", false).Valid {
		return zero, errors.New("panel is not connected and synchronized")
	}
	ctx, cancel := context.WithTimeout(ctx, c.config.CommandTimeout)
	defer cancel()
	r := request{ctx: ctx, kind: kind, value: value, reply: make(chan response, 1)}
	select {
	case c.commands <- r:
	case <-ctx.Done():
		return zero, fmt.Errorf("command not confirmed: %w", ctx.Err())
	}
	select {
	case result := <-r.reply:
		return result.reading, result.err
	case <-ctx.Done():
		return zero, fmt.Errorf("command not confirmed; output may have changed: %w", ctx.Err())
	}
}

// Run never replays an output command after reconnecting. GET is used only to synchronize.
func (c *Controller) Run(ctx context.Context) {
	for ctx.Err() == nil {
		c.store.Connecting()
		port, err := serialport.Open(c.config.Port, &serialport.Mode{
			BaudRate: c.config.Baud, DataBits: 8, Parity: serialport.NoParity, StopBits: serialport.OneStopBit,
		})
		if err == nil {
			err = port.SetReadTimeout(250 * time.Millisecond)
		}
		if err == nil {
			err = c.serve(ctx, port)
		}
		if port != nil {
			_ = port.Close()
		}
		if ctx.Err() != nil {
			c.store.Disconnected("server stopped")
			return
		}
		c.logger.Warn("serial connection ended", "error", err)
		c.store.Disconnected(err.Error())
		timer := time.NewTimer(c.config.ReconnectDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func readLines(ctx context.Context, port serialport.Port, lines chan<- string, failures chan<- error) {
	buffer := make([]byte, 256)
	line := make([]byte, 0, protocol.MaxLine)
	fail := func(err error) {
		select {
		case failures <- err:
		case <-ctx.Done():
		}
	}
	for ctx.Err() == nil {
		n, err := port.Read(buffer)
		// A read timeout returns (0, nil), not EOF.
		for _, b := range buffer[:n] {
			if b == '\n' {
				if len(line) > 0 {
					select {
					case lines <- string(line):
					case <-ctx.Done():
						return
					}
				}
				line = line[:0]
			} else {
				if len(line) >= protocol.MaxLine {
					fail(errors.New("serial line exceeds protocol limit"))
					return
				}
				line = append(line, b)
			}
		}
		if err != nil {
			fail(err)
			return
		}
	}
}

// Close also interrupts a stalled driver write on cancellation or timeout.
// A partial command is never retried on a new connection.
func writeAll(ctx context.Context, callerDone <-chan struct{}, port serialport.Port, data []byte) error {
	finished, watcherDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = port.Close()
		case <-callerDone:
			_ = port.Close()
		case <-finished:
		}
	}()
	defer func() { close(finished); <-watcherDone }()
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := port.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return errors.New("serial write made no valid progress")
		}
		data = data[n:]
	}
	return ctx.Err()
}

func (c *Controller) serve(ctx context.Context, port serialport.Port) (returnErr error) {
	readCtx, stopReader := context.WithCancel(ctx)
	lines, failures, readerDone := make(chan string, 64), make(chan error, 1), make(chan struct{})
	go func() { defer close(readerDone); readLines(readCtx, port, lines, failures) }()
	// Closing the port wakes Read on shutdown; wait so no reader survives a reconnect.
	defer func() { stopReader(); _ = port.Close(); <-readerDone }()
	boot := time.NewTimer(c.config.BootDelay)
	defer boot.Stop()
	bootChannel := boot.C
	var p *pending
	var deadline <-chan time.Time
	var commandTimer *time.Timer
	var lastSequence uint32
	var haveSequence, synced, gapAnnounced bool
	finish := func(result response) {
		if p != nil && p.request != nil {
			p.request.reply <- result
		}
		p, deadline = nil, nil
		if commandTimer != nil {
			commandTimer.Stop()
		}
	}
	defer func() {
		if p != nil {
			finish(response{err: fmt.Errorf("command not confirmed; output may have changed: %w", returnErr)})
		}
	}()
	start := func(r *request) error {
		if r != nil && r.ctx.Err() != nil {
			r.reply <- response{err: r.ctx.Err()}
			return nil
		}
		c.nextID++
		if c.nextID == 0 {
			c.nextID++
		}
		kind, value := "GET", ""
		if r != nil {
			kind, value = r.kind, r.value
		}
		line, err := protocol.Command(kind, c.nextID, value)
		if err != nil {
			return err
		}
		p = &pending{id: c.nextID, request: r}
		writeCtx, cancelWrite := context.WithTimeout(ctx, c.config.CommandTimeout)
		var callerDone <-chan struct{}
		if r != nil {
			callerDone = r.ctx.Done()
		}
		err = writeAll(writeCtx, callerDone, port, []byte(line))
		cancelWrite()
		if err != nil {
			return err
		}
		commandTimer = time.NewTimer(c.config.CommandTimeout)
		deadline = commandTimer.C
		return nil
	}
	for {
		var commands <-chan request
		var cancelled <-chan struct{}
		if synced && p == nil {
			commands = c.commands
		}
		if p != nil && p.request != nil {
			cancelled = p.request.ctx.Done()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-failures:
			return err
		case <-cancelled:
			return errors.New("command caller cancelled before acknowledgement")
		case <-deadline:
			return errors.New("firmware acknowledgement timed out")
		case <-bootChannel:
			bootChannel = nil
			if err := start(nil); err != nil {
				return err
			}
		case r := <-commands:
			if err := start(&r); err != nil {
				return err
			}
		case line := <-lines:
			frame, err := protocol.Parse(line)
			if err != nil {
				c.store.Report("protocol_error", err.Error(), 0)
				return fmt.Errorf("invalid firmware frame; resynchronization required: %w", err)
			}
			switch frame.Kind {
			case "READY":
				if p != nil {
					finish(response{err: errors.New("firmware restarted before acknowledgement; output result unknown")})
				}
				c.store.Connecting()
				haveSequence, synced, gapAnnounced = false, false, false
				boot.Stop()
				bootChannel = nil
				if err := start(nil); err != nil {
					return err
				}
			case "GAP":
				gapAnnounced = true
				c.store.Report("gap", "firmware transmit queue overflowed; some input transitions were lost", frame.Lost)
			case "ERR":
				c.store.Report("firmware_error", frame.Code, 0)
				if p != nil && p.id == frame.ID {
					if p.request == nil {
						return fmt.Errorf("firmware rejected synchronization: %s", frame.Code)
					}
					finish(response{err: fmt.Errorf("firmware rejected command: %s", frame.Code)})
				}
			case "STATE", "ACK":
				frame.Reading.Generation = c.store.View(nil, "", false).Generation
				seq := frame.Reading.Seq
				if haveSequence {
					delta := seq - lastSequence // Unsigned subtraction handles sequence wrap.
					if delta == 0 {
						continue
					}
					if delta > 1<<31 {
						return errors.New("serial sequence moved backwards without READY")
					}
					if delta > 1 && !gapAnnounced {
						c.store.Report("gap", "serial sequence gap; some input transitions were lost", delta-1)
					}
				}
				lastSequence, haveSequence, gapAnnounced = seq, true, false
				c.store.Apply(frame.Reading)
				if frame.Kind == "ACK" && p != nil && frame.ID == p.id {
					if p.request == nil {
						synced = true
						finish(response{reading: frame.Reading})
						continue
					}
					r := p.request
					matched := r.kind == "GET" ||
						(r.kind == "RGB" && frame.Reading.State.RGB == r.value) ||
						(r.kind == "RELAY" && fmt.Sprint(frame.Reading.State.Relay) == r.value)
					if !matched {
						finish(response{err: errors.New("ACK state does not match requested output")})
					} else {
						finish(response{reading: frame.Reading})
					}
				}
			}
		}
	}
}
