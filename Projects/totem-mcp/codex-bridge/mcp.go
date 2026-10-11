package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const eventsURI = "panel://events"

func listenPanel(ctx context.Context, endpoint string, c *cursor, emit func(change) error, observe func(document), connection func(bool, error)) error {
	for {
		err := panelSession(ctx, endpoint, c, emit, observe, connection)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Overflow stops the bridge rather than silently discarding presses.
		if errors.Is(err, errQueueFull) {
			return err
		}
		log.Printf("MCP savienojums: %v; atkārto pēc 2 s", err)
		t := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func panelSession(ctx context.Context, endpoint string, c *cursor, emit func(change) error, observe func(document), connection func(bool, error)) (sessionErr error) {
	var s *mcp.ClientSession
	defer func() {
		if connection != nil {
			connection(false, sessionErr)
		}
		if s != nil {
			_ = s.Close()
		}
	}()
	streamEnded := make(chan error, 1)
	updates, ack := make(chan struct{}, 1), make(chan struct{}, 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "totem-codex-bridge", Version: "1.0.0"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, r *mcp.ResourceUpdatedNotificationRequest) {
			if r != nil && r.Params != nil && r.Params.URI == eventsURI {
				select {
				case updates <- struct{}{}:
				default:
				}
			}
		},
	})
	client.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, r mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, r)
			if method == "notifications/subscriptions/acknowledged" && err == nil {
				if p, ok := r.GetParams().(*mcp.SubscriptionsAcknowledgedParams); ok && p != nil {
					for _, uri := range p.Notifications.ResourceSubscriptions {
						if uri == eventsURI {
							select {
							case ack <- struct{}{}:
							default:
							}
						}
					}
				}
			}
			return result, err
		}
	})
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	s, err := client.Connect(callCtx, &mcp.StreamableClientTransport{
		Endpoint: endpoint, MaxRetries: -1,
		HTTPClient: &http.Client{Transport: eventStreamTransport{base: http.DefaultTransport, ended: streamEnded}},
	}, nil)
	cancel()
	if err != nil {
		return err
	}
	d, err := readDocument(ctx, s)
	if err != nil {
		return err
	}
	if !c.initialized {
		c.baseline(d)
	} else if err = c.process(d, emit); err != nil {
		return err
	}
	if observe != nil {
		observe(d)
	}
	callCtx, cancel = context.WithTimeout(ctx, 15*time.Second)
	err = s.Subscribe(callCtx, &mcp.SubscribeParams{URI: eventsURI})
	if err == nil {
		select {
		case <-ack:
		case err = <-streamEnded:
		case <-callCtx.Done():
			err = callCtx.Err()
		}
	}
	cancel()
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	// Recover changes between the first read and subscription activation.
	d, err = readDocument(ctx, s)
	if err != nil {
		return err
	}
	if err = c.process(d, emit); err != nil {
		return err
	}
	if observe != nil {
		observe(d)
	}
	if connection != nil {
		connection(true, nil)
	}
	log.Printf("MCP abonēts: stream=%s event=%d", c.stream, c.id)
	ended := make(chan error, 1)
	go func() { ended <- s.Wait() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-streamEnded:
			return err
		case err := <-ended:
			if err == nil {
				return errors.New("session ended")
			}
			return err
		case <-updates:
			d, err := readDocument(ctx, s)
			if err != nil {
				return err
			}
			if err = c.process(d, emit); err != nil {
				return err
			}
			if observe != nil {
				observe(d)
			}
		}
	}
}

// The SDK can retain a logical session after subscriptions/listen ends. Watch
// the actual event stream so a dead subscription cannot leave the UI connected.
// Ordinary finite SSE responses (such as resources/read) are not monitored.
type eventStreamTransport struct {
	base  http.RoundTripper
	ended chan<- error
}

func (t eventStreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" && isSubscriptionRequest(req) {
		resp.Body = &eventStreamBody{ReadCloser: resp.Body, ctx: req.Context(), ended: t.ended}
	}
	return resp, nil
}

func isSubscriptionRequest(req *http.Request) bool {
	if req.Method == http.MethodGet {
		return true // Legacy standalone notification stream.
	}
	if req.Method != http.MethodPost || req.GetBody == nil {
		return false
	}
	body, err := req.GetBody()
	if err != nil {
		return false
	}
	defer body.Close()
	var call struct{ Method string }
	return json.NewDecoder(body).Decode(&call) == nil && call.Method == "subscriptions/listen"
}

type eventStreamBody struct {
	io.ReadCloser
	ctx   context.Context
	ended chan<- error
}

func (b *eventStreamBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && b.ctx.Err() == nil {
		select {
		case b.ended <- fmt.Errorf("MCP notikumu plūsma pārtrūkusi: %w", err):
		default:
		}
	}
	return n, err
}

func readDocument(ctx context.Context, s *mcp.ClientSession) (document, error) {
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, err := s.ReadResource(callCtx, &mcp.ReadResourceParams{URI: eventsURI})
	if err != nil {
		return document{}, err
	}
	if r != nil {
		for _, content := range r.Contents {
			if content != nil && content.URI == eventsURI && content.Text != "" {
				var d document
				if err = json.Unmarshal([]byte(content.Text), &d); err != nil {
					return d, err
				}
				if d.StreamID == "" {
					return d, errors.New("empty stream_id")
				}
				return d, nil
			}
		}
	}
	return document{}, errors.New("panel://events JSON content missing")
}
