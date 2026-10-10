package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"totem-mcp/internal/panel"
)

type fakeCommander struct {
	mu    sync.Mutex
	calls int
}

func (c *fakeCommander) Execute(_ context.Context, kind, value string) (panel.Reading, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	r := panel.Reading{State: panel.State{RGB: "#000000"}, Seq: 42, Generation: 11}
	if kind == "RGB" {
		r.State.RGB = value
	}
	if kind == "RELAY" && value == "1" {
		r.State.Relay = 1
	}
	return r, nil
}

func connectTest(t *testing.T, server *sdk.Server, clientOptions *sdk.ClientOptions) (context.Context, *sdk.ClientSession, <-chan string) {
	t.Helper()
	return connectTestVersion(t, server, clientOptions, ProtocolVersion)
}

func connectTestVersion(t *testing.T, server *sdk.Server, clientOptions *sdk.ClientOptions, version string) (context.Context, *sdk.ClientSession, <-chan string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	h := NewHTTPHandler(server, testLogger())
	host := httptest.NewServer(h)
	client := sdk.NewClient(&sdk.Implementation{Name: "totem-tests", Version: "0"}, clientOptions)
	acknowledged := make(chan string, 8)
	client.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			result, err := next(ctx, method, req)
			if method == "notifications/subscriptions/acknowledged" && err == nil {
				if params, ok := req.GetParams().(*sdk.SubscriptionsAcknowledgedParams); ok {
					for _, uri := range params.Notifications.ResourceSubscriptions {
						select {
						case acknowledged <- uri:
						default:
						}
					}
				}
			}
			return result, err
		}
	})
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, &sdk.ClientSessionOptions{ProtocolVersion: version})
	if err != nil {
		cancel()
		host.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close(); cancel(); host.Close() })
	if session.InitializeResult().ProtocolVersion != version {
		t.Fatal("wrong negotiated protocol version")
	}
	return ctx, session, acknowledged
}

func TestLegacyClientsReadToolsAndReceiveUpdates(t *testing.T) {
	for _, version := range []string{"2025-11-25", "2025-06-18", "2025-03-26"} {
		t.Run(version, func(t *testing.T) {
			store := newTestStore(t)
			store.Connecting()
			reading := panel.Reading{State: panel.State{RGB: "#000000"}, Seq: 1}
			store.Apply(reading)
			server := New(store, &fakeCommander{}, testLogger())
			updated := make(chan string, 4)
			ctx, client, _ := connectTestVersion(t, server, &sdk.ClientOptions{
				ResourceUpdatedHandler: func(_ context.Context, req *sdk.ResourceUpdatedNotificationRequest) {
					select {
					case updated <- req.Params.URI:
					default:
					}
				},
			}, version)
			tools, err := client.ListTools(ctx, &sdk.ListToolsParams{})
			if err != nil {
				t.Fatal(err)
			}
			if len(tools.Tools) != 3 {
				t.Fatalf("legacy client did not receive all tools: %+v", tools)
			}
			result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "get_panel_state", Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			var view panel.View
			decodeResult(t, result, &view)
			if !view.Valid || view.State == nil || view.State.Button1 != 0 {
				t.Fatalf("legacy client could not read state: %+v", view)
			}
			// The legacy Subscribe response confirms activation synchronously.
			if err := client.Subscribe(ctx, &sdk.SubscribeParams{URI: EventsURI}); err != nil {
				t.Fatal(err)
			}
			reading.Seq, reading.State.Button1 = 2, 1
			store.Apply(reading)
			if err := server.ResourceUpdated(ctx, &sdk.ResourceUpdatedNotificationParams{URI: EventsURI}); err != nil {
				t.Fatal(err)
			}
			select {
			case uri := <-updated:
				if uri != EventsURI {
					t.Fatalf("wrong resource updated: %s", uri)
				}
			case <-ctx.Done():
				t.Fatal("legacy subscription did not receive a notification")
			}
			resource, err := client.ReadResource(ctx, &sdk.ReadResourceParams{URI: EventsURI})
			if err != nil {
				t.Fatal(err)
			}
			if len(resource.Contents) != 1 {
				t.Fatal("missing legacy event resource")
			}
			if err := json.Unmarshal([]byte(resource.Contents[0].Text), &view); err != nil {
				t.Fatal(err)
			}
			if view.State == nil || view.State.Button1 != 1 {
				t.Fatal("legacy resource read did not contain the new state")
			}
		})
	}
}

func newTestStore(t *testing.T) *panel.Store {
	t.Helper()
	s, err := panel.New("test", 115200, 16)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func decodeResult(t *testing.T, result *sdk.CallToolResult, target any) {
	t.Helper()
	if result.IsError {
		t.Fatalf("unexpected tool error: %+v", result.Content)
	}
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, target); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineStateAndOutputToolsOverHTTP(t *testing.T) {
	store, commands := newTestStore(t), &fakeCommander{}
	ctx, client, _ := connectTest(t, New(store, commands, testLogger()), nil)
	result, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "get_panel_state", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var view panel.View
	decodeResult(t, result, &view)
	if view.Valid || view.State != nil || view.LastUpdate != nil || view.Status != "disconnected" {
		t.Fatalf("offline state was invented: %+v", view)
	}
	result, err = client.CallTool(ctx, &sdk.CallToolParams{Name: "set_rgb", Arguments: map[string]any{"color": "#00FF00\nRELAY=1"}})
	if err != nil {
		t.Fatal(err)
	}
	commands.mu.Lock()
	calls := commands.calls
	commands.mu.Unlock()
	if !result.IsError || calls != 0 {
		t.Fatal("malformed RGB reached the command executor")
	}
	result, err = client.CallTool(ctx, &sdk.CallToolParams{Name: "set_relay", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("missing on argument silently turned the relay off")
	}
	result, err = client.CallTool(ctx, &sdk.CallToolParams{Name: "set_rgb", Arguments: map[string]any{"color": "#00ff00"}})
	if err != nil {
		t.Fatal(err)
	}
	var output OutputResult
	decodeResult(t, result, &output)
	if !output.Confirmed || output.State.RGB != "#00FF00" || output.Sequence != 42 || output.Generation != 11 {
		t.Fatalf("tool did not return its confirmed reading: %+v", output)
	}
}

func TestSubscribedNotificationRetainsBothButtonTransitions(t *testing.T) {
	store := newTestStore(t)
	changes := make(chan struct{}, 1)
	store.SetOnChange(func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	})
	store.Connecting()
	r := panel.Reading{State: panel.State{RGB: "#000000"}, Seq: 1}
	store.Apply(r)
	server := New(store, &fakeCommander{}, testLogger())
	updated := make(chan string, 4)
	ctx, client, acknowledged := connectTest(t, server, &sdk.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *sdk.ResourceUpdatedNotificationRequest) {
			select {
			case updated <- req.Params.URI:
			default:
			}
		},
	})
	if err := client.Subscribe(ctx, &sdk.SubscribeParams{URI: EventsURI}); err != nil {
		t.Fatal(err)
	}
	// Subscribe starts a stream asynchronously; wait for the standard acknowledgement.
	select {
	case uri := <-acknowledged:
		if uri != EventsURI {
			t.Fatalf("wrong subscription acknowledged: %s", uri)
		}
	case <-ctx.Done():
		t.Fatal("subscription was not acknowledged")
	}
	r.Seq, r.State.Button1 = 2, 1
	store.Apply(r)
	r.Seq, r.State.Button1 = 3, 0
	store.Apply(r)
	// Start delivery only after both transitions, deliberately coalescing notifications.
	go Notify(ctx, server, changes, testLogger())
	select {
	case uri := <-updated:
		if uri != EventsURI {
			t.Fatalf("unexpected notification: %s", uri)
		}
	case <-ctx.Done():
		t.Fatal("subscription did not receive an update")
	}
	resource, err := client.ReadResource(ctx, &sdk.ReadResourceParams{URI: EventsURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(resource.Contents) != 1 {
		t.Fatal("missing event resource")
	}
	var view panel.View
	if err := json.Unmarshal([]byte(resource.Contents[0].Text), &view); err != nil {
		t.Fatal(err)
	}
	n := len(view.Events)
	if n < 2 || view.State == nil || view.State.Button1 != 0 || view.Events[n-2].State.Button1 != 1 || view.Events[n-1].State.Button1 != 0 {
		t.Fatalf("short press disappeared from event history: %+v", view)
	}
}
