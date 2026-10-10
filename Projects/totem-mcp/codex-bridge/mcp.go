package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const eventsURI = "panel://events"

func listenPanel(ctx context.Context, endpoint string, c *cursor, emit func(change) error, observe func(document)) error {
	for {
		err := panelSession(ctx, endpoint, c, emit, observe)
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

func panelSession(ctx context.Context, endpoint string, c *cursor, emit func(change) error, observe func(document)) error {
	updates, ack := make(chan struct{}, 1), make(chan struct{}, 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "totem-codex-bridge", Version: "0.1.0"}, &mcp.ClientOptions{
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
	s, err := client.Connect(callCtx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	cancel()
	if err != nil {
		return err
	}
	defer s.Close()
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
	log.Printf("MCP abonēts: stream=%s event=%d", c.stream, c.id)
	ended := make(chan error, 1)
	go func() { ended <- s.Wait() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
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
