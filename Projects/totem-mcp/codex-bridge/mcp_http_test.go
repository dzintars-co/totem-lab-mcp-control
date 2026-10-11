//go:build go1.27

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Exercise the full HTTP/SSE protocol without binding a network port. These
// tests run sequentially and stop their clients before restoring the transport.
func newMCPTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewTestServer(t, handler)
	client := server.Client()
	previous := http.DefaultTransport
	http.DefaultTransport = client.Transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	return server
}
