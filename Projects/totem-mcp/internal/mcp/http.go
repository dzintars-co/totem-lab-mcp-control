package mcp

import (
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewHTTPHandler preserves the sessionless 2026 protocol while also serving
// clients that use initialize and persistent sessions from the 2025 revisions.
// New-protocol requests must carry MCP-Protocol-Version; legacy initialize
// requests may omit it. The official SDK validates versions and messages.
func NewHTTPHandler(server *sdk.Server, logger *slog.Logger) http.Handler {
	getServer := func(*http.Request) *sdk.Server { return server }
	current := sdk.NewStreamableHTTPHandler(getServer, &sdk.StreamableHTTPOptions{
		Stateless: true, Logger: logger, MaxRequestBodyBytes: 1 << 20,
		PropagateRequestCancellation: true,
	})
	legacy := sdk.NewStreamableHTTPHandler(getServer, &sdk.StreamableHTTPOptions{
		Stateless: false, Logger: logger, MaxRequestBodyBytes: 1 << 20,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("MCP-Protocol-Version") >= ProtocolVersion {
			current.ServeHTTP(w, r)
		} else {
			legacy.ServeHTTP(w, r)
		}
	})
}
