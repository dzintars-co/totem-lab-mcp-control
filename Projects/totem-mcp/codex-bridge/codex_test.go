package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary acts as Codex CLI. It never loads credentials or contacts AI.
func TestMain(m *testing.M) {
	if mode := os.Getenv("TOTEM_CODEX_FIXTURE"); mode != "" {
		prompt, _ := io.ReadAll(os.Stdin)
		if !strings.Contains(string(prompt), "Totem MCP") {
			os.Exit(5)
		}
		if mode == "timeout" {
			time.Sleep(10 * time.Second)
			os.Exit(1)
		}
		if mode == "malformed" {
			os.Stdout.WriteString("not json\n")
			os.Exit(0)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.Encode(map[string]any{"type": "thread.started", "thread_id": "fixture-thread"})
		if mode == "fail" {
			enc.Encode(map[string]any{"type": "turn.failed", "error": map[string]any{"message": "fixture failure"}})
			os.Exit(0)
		}
		if mode == "incomplete" {
			os.Exit(0)
		}
		enc.Encode(map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": "Poga A nospiesta."}})
		enc.Encode(map[string]any{"type": "turn.completed"})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCodexCLIStartsResumesAndPrints(t *testing.T) {
	t.Setenv("TOTEM_CODEX_FIXTURE", "ok")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var out bytes.Buffer
	r := &codexRunner{cli: exe, cwd: dir, threadFile: filepath.Join(dir, "thread.txt"), instruction: defaultInstruction, timeout: 5 * time.Second, out: &out, stderr: io.Discard}
	if err := r.deliver(context.Background(), []change{{Kind: "state", EventID: 2}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(r.threadFile)
	if err != nil || string(b) != "fixture-thread\n" {
		t.Fatalf("thread not saved: %s %v", b, err)
	}
	args := strings.Join(r.args(), " ")
	if !strings.Contains(args, "resume fixture-thread") || strings.Contains(args, "--last") {
		t.Fatal(args)
	}
	if err := r.deliver(context.Background(), []change{{Kind: "state", EventID: 3}}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "Poga A nospiesta.") != 2 {
		t.Fatal(out.String())
	}
}

func TestCodexCLIRejectsFailureAndTimeout(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"fail", "incomplete", "malformed", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("TOTEM_CODEX_FIXTURE", mode)
			r := &codexRunner{cli: exe, cwd: t.TempDir(), instruction: defaultInstruction, timeout: 100 * time.Millisecond, out: io.Discard, stderr: io.Discard}
			if err := r.deliver(context.Background(), nil); err == nil {
				t.Fatal("unconfirmed delivery accepted")
			}
		})
	}
}
