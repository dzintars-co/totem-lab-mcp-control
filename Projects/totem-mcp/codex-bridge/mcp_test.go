package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPSubscriptionReadsCoalescedPressAndRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	doc := baselineDoc()
	read := make(chan struct{}, 10)
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{
		SupportedProtocolVersions: []string{"2026-07-28"},
		SubscribeHandler:          func(context.Context, *mcp.SubscribeRequest) error { return nil },
		UnsubscribeHandler:        func(context.Context, *mcp.UnsubscribeRequest) error { return nil },
	})
	server.AddResource(&mcp.Resource{URI: eventsURI, Name: "events", MIMEType: "application/json"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		mu.Lock()
		b, err := json.Marshal(doc)
		mu.Unlock()
		select {
		case read <- struct{}{}:
		default:
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: eventsURI, Text: string(b)}}}, err
	})
	host := newMCPTestServer(t, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer host.Close()
	got := make(chan change, 10)
	observed := make(chan document, 10)
	done := make(chan error, 1)
	go func() {
		done <- panelSession(ctx, host.URL, &cursor{potStep: 2}, func(x change) error { got <- x; return nil }, func(d document) { observed <- d }, nil)
	}()
	// Initial read and read after subscription acknowledgement.
	for i := 0; i < 2; i++ {
		select {
		case <-read:
		case <-ctx.Done():
			t.Fatal("subscription not ready")
		}
	}
	select {
	case snapshot := <-observed:
		if !snapshot.Valid || snapshot.State == nil || snapshot.StreamID != "test" {
			t.Fatalf("invalid initial dashboard snapshot: %+v", snapshot)
		}
	case <-ctx.Done():
		t.Fatal("initial snapshot was not observed")
	}
	mu.Lock()
	doc.LastEventID = 3
	doc.Events = []panelEvent{{ID: 2, Generation: 1, Kind: "state", State: &state{Button3: 1}}, {ID: 3, Generation: 1, Kind: "state", State: &state{}}}
	mu.Unlock()
	if err := server.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: eventsURI}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{1, 0} {
		select {
		case x := <-got:
			if x.State == nil || x.State.Button3 != want || x.EventID != uint64(i+2) {
				t.Fatalf("wrong transition: %+v", x)
			}
		case <-ctx.Done():
			t.Fatal("notification did not reach bridge")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("listener did not stop")
	}
}

func TestMCPDisconnectAndReconnectWithoutPanelChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	server := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, &mcp.ServerOptions{
		SupportedProtocolVersions: []string{"2026-07-28"},
		SubscribeHandler:          func(context.Context, *mcp.SubscribeRequest) error { return nil },
		UnsubscribeHandler:        func(context.Context, *mcp.UnsubscribeRequest) error { return nil },
	})
	server.AddResource(&mcp.Resource{URI: eventsURI, Name: "events"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		b, err := json.Marshal(baselineDoc())
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: eventsURI, Text: string(b)}}}, err
	})
	host := newMCPTestServer(t, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer host.Close()
	connections := make(chan mcpConnectionMsg, 10)
	done := make(chan error, 1)
	go func() {
		done <- listenPanel(ctx, host.URL, &cursor{potStep: 2}, func(change) error {
			t.Error("transport reconnect emitted a hardware change")
			return nil
		}, nil, func(connected bool, err error) {
			connections <- mcpConnectionMsg{connected: connected, err: err}
		})
	}()
	waitConnection := func(want bool) mcpConnectionMsg {
		t.Helper()
		select {
		case got := <-connections:
			if got.connected != want {
				t.Fatalf("connection=%v want=%v (error=%v)", got.connected, want, got.err)
			}
			return got
		case <-time.After(time.Second):
			t.Fatalf("connection status %v was not reported promptly", want)
			return mcpConnectionMsg{}
		}
	}
	waitConnection(true)
	// Only cut the HTTP streams. The server stays available for reconnection,
	// and the hardware state stays unchanged throughout the test.
	host.CloseClientConnections()
	if got := waitConnection(false); got.err == nil {
		t.Fatal("disconnect had no error")
	}
	select {
	case got := <-connections:
		if !got.connected || got.err != nil {
			t.Fatalf("reconnect did not restore subscription: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("bridge did not reconnect")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("listener stopped with %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("listener did not stop after cancellation")
	}
}

func TestMCPInitialConnectionFailureIsObserved(t *testing.T) {
	host := newMCPTestServer(t, http.NotFoundHandler())
	endpoint := host.URL
	host.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var statuses []mcpConnectionMsg
	err := panelSession(ctx, endpoint, &cursor{}, func(change) error { return nil }, nil,
		func(connected bool, err error) {
			statuses = append(statuses, mcpConnectionMsg{connected: connected, err: err})
		})
	if err == nil || len(statuses) != 1 || statuses[0].connected || statuses[0].err == nil {
		t.Fatalf("startup failure not observed: err=%v statuses=%+v", err, statuses)
	}
}
