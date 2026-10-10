package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	host := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true}))
	defer host.Close()
	got := make(chan change, 10)
	observed := make(chan document, 10)
	done := make(chan error, 1)
	go func() {
		done <- panelSession(ctx, host.URL, &cursor{potStep: 2}, func(x change) error { got <- x; return nil }, func(d document) { observed <- d })
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
