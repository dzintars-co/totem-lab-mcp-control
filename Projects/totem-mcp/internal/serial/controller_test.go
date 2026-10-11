package serial

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	serialport "go.bug.st/serial"
	"totem-mcp/internal/panel"
)

// Embed unused interface methods: these tests never open a real serial device.
type fakePort struct {
	serialport.Port
	reader *io.PipeReader
	device *io.PipeWriter
	writes chan string
	closed chan struct{}
	once   sync.Once
}

func newFakePort() *fakePort {
	r, w := io.Pipe()
	return &fakePort{reader: r, device: w, writes: make(chan string, 8), closed: make(chan struct{})}
}
func (p *fakePort) Read(b []byte) (int, error) { return p.reader.Read(b) }
func (p *fakePort) Write(b []byte) (int, error) {
	select {
	case p.writes <- string(b):
		return len(b), nil
	case <-p.closed:
		return 0, io.ErrClosedPipe
	}
}
func (p *fakePort) Close() error {
	p.once.Do(func() { close(p.closed); _ = p.reader.Close(); _ = p.device.Close() })
	return nil
}

func nextCommand(t *testing.T, ctx context.Context, p *fakePort, want string) uint32 {
	t.Helper()
	select {
	case line := <-p.writes:
		parts := strings.Split(strings.TrimSuffix(line, "\n"), ";")
		if len(parts) < 3 || parts[0] != want || parts[1] != "V=1" {
			t.Fatalf("unexpected command: %q", line)
		}
		id, err := strconv.ParseUint(strings.TrimPrefix(parts[2], "ID="), 10, 32)
		if err != nil || id == 0 {
			t.Fatalf("invalid command ID: %q", line)
		}
		return uint32(id)
	case <-ctx.Done():
		t.Fatal("no command written")
		return 0
	}
}

func feed(t *testing.T, p *fakePort, kind string, seq, id uint32, rgb string) {
	t.Helper()
	line := fmt.Sprintf("%s;V=1;SEQ=%d;SW1=0;SW2=0;SW3=0;B1=0;B2=0;B3=0;P1=0;P2=0;P3=0;ENC=0;EB=0;RGB=%s;RELAY=0", kind, seq, rgb)
	if kind == "ACK" {
		line += fmt.Sprintf(";ID=%d", id)
	}
	if _, err := io.WriteString(p.device, line+"\n"); err != nil {
		t.Fatal(err)
	}
}

func waitSequence(t *testing.T, ctx context.Context, s *panel.Store, seq uint32) {
	t.Helper()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for s.View(nil, "", false).Sequence != seq {
		select {
		case <-ctx.Done():
			t.Fatal("reading not processed")
		case <-tick.C:
		}
	}
}

func setupController(t *testing.T) (context.Context, *Controller, *fakePort, <-chan error) {
	return setupControllerWithProbe(t, time.Minute)
}

func setupControllerWithProbe(t *testing.T, interval time.Duration) (context.Context, *Controller, *fakePort, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	s, err := panel.New("test", 115200, 16)
	if err != nil {
		t.Fatal(err)
	}
	s.Connecting()
	c := New(Config{BootDelay: 0, CommandTimeout: time.Second, IdleProbeInterval: interval}, s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p, done := newFakePort(), make(chan error, 1)
	go func() { done <- c.serve(ctx, p); close(done) }()
	t.Cleanup(func() {
		cancel()
		_ = p.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("serial worker survived shutdown")
		}
	})
	id := nextCommand(t, ctx, p, "GET")
	feed(t, p, "ACK", 1, id, "#000000")
	waitSequence(t, ctx, s, 1)
	return ctx, c, p, done
}

func TestOutputRequiresMatchingAcknowledgement(t *testing.T) {
	ctx, c, p, _ := setupController(t)
	result := make(chan response, 1)
	go func() { r, err := c.Execute(ctx, "RGB", "#ff0000"); result <- response{r, err} }()
	id := nextCommand(t, ctx, p, "RGB")
	feed(t, p, "STATE", 2, 0, "#FF0000")
	feed(t, p, "ACK", 3, id+1, "#FF0000")
	waitSequence(t, ctx, c.store, 3)
	select {
	case r := <-result:
		t.Fatalf("unmatched ACK confirmed command: %+v", r)
	default:
	}
	feed(t, p, "ACK", 4, id, "#FF0000")
	select {
	case r := <-result:
		if r.err != nil || r.reading.State.RGB != "#FF0000" || r.reading.Seq != 4 || r.reading.Generation != 1 {
			t.Fatalf("matching ACK was not returned: %+v", r)
		}
	case <-ctx.Done():
		t.Fatal("matching ACK did not complete command")
	}
}

func TestWrongOutputInMatchingACKIsFailure(t *testing.T) {
	ctx, c, p, _ := setupController(t)
	result := make(chan error, 1)
	go func() { _, err := c.Execute(ctx, "RGB", "#FF0000"); result <- err }()
	id := nextCommand(t, ctx, p, "RGB")
	feed(t, p, "ACK", 2, id, "#000000")
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("wrong output was confirmed")
		}
	case <-ctx.Done():
		t.Fatal("command did not complete")
	}
}

func TestMissingACKNeverReportsSuccess(t *testing.T) {
	ctx, c, p, _ := setupController(t)
	callCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := c.Execute(callCtx, "RELAY", "1"); result <- err }()
	_ = nextCommand(t, ctx, p, "RELAY")
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("missing ACK was reported as success")
		}
	case <-ctx.Done():
		t.Fatal("cancelled command did not complete")
	}
}

func TestMalformedFrameRequiresResynchronization(t *testing.T) {
	ctx, c, p, done := setupController(t)
	if _, err := io.WriteString(p.device, "STATE;V=1;SEQ=2;B1=1\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "resynchronization required") {
			t.Fatalf("malformed frame did not end the serial connection: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("serial worker kept using the connection after a malformed frame")
	}
	v := c.store.View(nil, "", true)
	if v.Sequence != 1 || v.LastError == "" || v.Events[len(v.Events)-1].Kind != "protocol_error" {
		t.Fatalf("malformed frame changed the reading or lost its error: %+v", v)
	}
}

func TestIdleFirmwareRepliesDoNotInventInputEvents(t *testing.T) {
	ctx, c, p, _ := setupControllerWithProbe(t, 20*time.Millisecond)
	initial := c.store.View(nil, "", true)
	for seq := uint32(2); seq <= 4; seq++ {
		id := nextCommand(t, ctx, p, "GET")
		feed(t, p, "ACK", seq, id, "#000000")
		waitSequence(t, ctx, c.store, seq)
		view := c.store.View(nil, "", true)
		if !view.Valid || view.Status != "connected" || view.LastEventID != initial.LastEventID {
			t.Fatalf("idle firmware became disconnected or created an input event: %+v", view)
		}
	}
}

func TestSilentFirmwareEndsConnectionAfterIdleProbe(t *testing.T) {
	ctx, _, p, done := setupControllerWithProbe(t, 20*time.Millisecond)
	_ = nextCommand(t, ctx, p, "GET")
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "firmware acknowledgement timed out") {
			t.Fatalf("silent firmware did not end the connection: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("silent firmware connection was retained indefinitely")
	}
	select {
	case <-p.closed:
	default:
		t.Fatal("unresponsive firmware port was not closed")
	}
	select {
	case line := <-p.writes:
		t.Fatalf("command was replayed after an unanswered idle probe: %q", line)
	default:
	}
}

func TestIdleProbeDoesNotInterruptOutputAcknowledgement(t *testing.T) {
	ctx, c, p, _ := setupControllerWithProbe(t, 100*time.Millisecond)
	result := make(chan response, 1)
	go func() { r, err := c.Execute(ctx, "RGB", "#FF0000"); result <- response{r, err} }()
	id := nextCommand(t, ctx, p, "RGB")
	select {
	case line := <-p.writes:
		t.Fatalf("idle probe interrupted an output command: %q", line)
	case <-time.After(300 * time.Millisecond):
	}
	feed(t, p, "ACK", 2, id, "#FF0000")
	select {
	case r := <-result:
		if r.err != nil || r.reading.State.RGB != "#FF0000" {
			t.Fatalf("output was not confirmed before the idle probe: %+v", r)
		}
	case <-ctx.Done():
		t.Fatal("output acknowledgement was not processed")
	}
	probeID := nextCommand(t, ctx, p, "GET")
	if probeID == id {
		t.Fatal("idle probe reused the output command ID")
	}
	feed(t, p, "ACK", 3, probeID, "#FF0000")
	waitSequence(t, ctx, c.store, 3)
	if view := c.store.View(nil, "", false); !view.Valid || view.State.RGB != "#FF0000" {
		t.Fatalf("idle probe changed the confirmed output: %+v", view)
	}
}
