//go:build !go1.27

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Go 1.26 does not yet provide the in-memory httptest server.
func newMCPTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}
