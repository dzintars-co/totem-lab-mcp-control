package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var errQueueFull = errors.New("notikumu rinda pilna; apturēts, lai klusi nepazaudētu notikumus")

type options struct {
	endpoint, cli, cwd, threadFile, instructionFile string
	dry                                             bool
	ui                                              bool
	potStep                                         float64
	batchWindow, turnTimeout                        time.Duration
}

func main() {
	var o options
	flag.StringVar(&o.endpoint, "mcp-url", "http://127.0.0.1:8080/mcp", "Totem MCP URL")
	flag.StringVar(&o.cli, "codex", "codex", "Codex CLI executable")
	flag.StringVar(&o.cwd, "cwd", ".", "Codex darba mape")
	flag.StringVar(&o.threadFile, "thread-file", "thread-id.txt", "savienojuma Codex sarunas ID fails")
	flag.StringVar(&o.instructionFile, "instructions", "", "papildu lietotāja instrukcijas fails")
	flag.BoolVar(&o.dry, "dry-run", false, "parādīt MCP notikumus, nepalaižot Codex")
	flag.BoolVar(&o.ui, "ui", false, "rādīt Bubble Tea termināļa paneli")
	flag.Float64Var(&o.potStep, "pot-step", 2, "potenciometra izmaiņas slieksnis procentpunktos (0 = katra izmaiņa)")
	flag.DurationVar(&o.batchWindow, "batch-window", 200*time.Millisecond, "laiks notikumu grupēšanai")
	flag.DurationVar(&o.turnTimeout, "turn-timeout", 2*time.Minute, "vienas Codex atbildes laika limits")
	flag.Parse()
	if err := validateOptions(o); err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runProgram := run
	if o.ui {
		runProgram = runDashboard
	}
	if err := runProgram(ctx, o, os.Stdout, os.Stderr); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func validateOptions(o options) error {
	u, err := url.Parse(o.endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("mcp-url jābūt pilnam HTTP(S) URL")
	}
	if math.IsNaN(o.potStep) || math.IsInf(o.potStep, 0) || o.potStep < 0 || o.potStep > 100 {
		return errors.New("pot-step jābūt 0..100")
	}
	if o.batchWindow < 0 || o.batchWindow > 10*time.Second {
		return errors.New("batch-window jābūt 0..10s")
	}
	if o.turnTimeout <= 0 {
		return errors.New("turn-timeout jābūt pozitīvam")
	}
	if strings.TrimSpace(o.cli) == "" || strings.TrimSpace(o.threadFile) == "" {
		return errors.New("codex un thread-file nedrīkst būt tukši")
	}
	return nil
}

func run(parent context.Context, o options, out, stderr io.Writer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cwd, err := filepath.Abs(o.cwd)
	if err != nil {
		return err
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("cwd nav mape")
	}
	r := &codexRunner{cli: o.cli, cwd: cwd, instruction: defaultInstruction, timeout: o.turnTimeout, out: out, stderr: stderr}
	if !o.dry {
		r.threadFile, err = filepath.Abs(o.threadFile)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(r.threadFile)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		r.thread = strings.TrimSpace(string(b))
		if strings.ContainsAny(r.thread, "\r\n\t ") || strings.HasPrefix(r.thread, "-") {
			return errors.New("invalid thread ID file")
		}
		if o.instructionFile != "" {
			b, err = os.ReadFile(o.instructionFile)
			if err != nil {
				return err
			}
			r.instruction += "\n\nLietotāja papildu instrukcija:\n" + string(b)
		}
	}
	q := make(chan change, 4096)
	emit := func(x change) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		select {
		case q <- x:
			return nil
		default:
			return errQueueFull
		}
	}
	var listenErr error
	listenerStopped := make(chan struct{})
	go func() {
		listenErr = listenPanel(ctx, o.endpoint, &cursor{potStep: o.potStep}, emit, nil, nil)
		close(listenerStopped)
	}()
	defer func() { cancel(); <-listenerStopped }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-listenerStopped:
			return listenErr
		case first := <-q:
			batch := []change{first}
			t := time.NewTimer(o.batchWindow)
		collect:
			for len(batch) < 64 {
				select {
				case x := <-q:
					batch = append(batch, x)
				case <-t.C:
					break collect
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-listenerStopped:
					t.Stop()
					return listenErr
				}
			}
			t.Stop()
			if o.dry {
				b, err := encodeChanges(batch)
				if err != nil {
					return err
				}
				if _, err = fmt.Fprintln(out, b); err != nil {
					return err
				}
			} else {
				log.Printf("Codex saņem %d notikumus", len(batch))
				if err = r.deliver(ctx, batch); err != nil {
					return err
				}
			}
		}
	}
}
