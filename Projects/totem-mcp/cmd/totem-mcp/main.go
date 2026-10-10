package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	panelmcp "totem-mcp/internal/mcp"
	"totem-mcp/internal/panel"
	panelserial "totem-mcp/internal/serial"
)

func main() {
	if err := run(); err != nil {
		slog.Error("totem-mcp stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.String("port", "", "serial device path (required)")
	baud := flag.Int("baud", 115200, "must match firmware BAUD_RATE")
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address; MCP endpoint is /mcp")
	bootDelay := flag.Duration("boot-delay", 2*time.Second, "wait for board startup after opening serial")
	reconnect := flag.Duration("reconnect-delay", 2*time.Second, "delay between serial reconnect attempts")
	commandTimeout := flag.Duration("command-timeout", 2*time.Second, "maximum time to confirm a command")
	history := flag.Int("event-history", 256, "number of retained events (16..4096)")
	flag.Parse()
	if *port == "" {
		return errors.New("-port is required; choose the actual device path")
	}
	if *baud <= 0 || *bootDelay < 0 || *reconnect <= 0 || *commandTimeout <= 0 || *history < 16 || *history > 4096 {
		return errors.New("invalid baud, timing or event-history option")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("listen address: %w", err)
	}
	token := os.Getenv("TOTEM_MCP_TOKEN")
	ip := net.ParseIP(host)
	local := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !local && token == "" {
		return errors.New("non-loopback listening requires TOTEM_MCP_TOKEN")
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	store, err := panel.New(*port, *baud, *history)
	if err != nil {
		return err
	}
	changes := make(chan struct{}, 1)
	store.SetOnChange(func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	})
	controller := panelserial.New(panelserial.Config{
		Port: *port, Baud: *baud, BootDelay: *bootDelay, ReconnectDelay: *reconnect, CommandTimeout: *commandTimeout,
	}, store, logger)
	server := panelmcp.New(store, controller, logger)
	var handler http.Handler = panelmcp.NewHTTPHandler(server, logger)
	handler = http.NewCrossOriginProtection().Handler(handler)
	if token != "" {
		handler = bearer(token, handler)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	httpServer := &http.Server{
		Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		// subscriptions/listen is intentionally long-lived: no HTTP WriteTimeout.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	serialDone := make(chan struct{})
	go func() { defer close(serialDone); controller.Run(ctx) }()
	go panelmcp.Notify(ctx, server, changes, logger)
	failures := make(chan error, 1)
	go func() { failures <- httpServer.Serve(listener) }()
	fmt.Fprintf(os.Stderr, "totem-mcp: http://%s/mcp; MCP %s; serial %s at %d baud\n", listener.Addr(), panelmcp.ProtocolVersion, *port, *baud)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-failures:
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
	}
	cancel()
	shutdown, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdown); err != nil {
		_ = httpServer.Close()
	}
	<-serialDone
	return serveErr
}

// An optional fixed bearer token is local deployment configuration, not an OAuth server.
func bearer(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
