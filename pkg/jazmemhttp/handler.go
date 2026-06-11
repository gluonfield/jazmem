// Package jazmemhttp exposes jazmem's HTTP surfaces for embedding hosts.
package jazmemhttp

import (
	"net/http"

	"github.com/gluonfield/jazmem/internal/httpapi"
	"github.com/gluonfield/jazmem/internal/mcpapi"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewMCPServer returns the in-process MCP server for one Memory.
func NewMCPServer(m *jazmem.Memory) *mcp.Server {
	return mcpapi.New(m)
}

// NewMCPHandler serves the MCP streamable HTTP protocol for one Memory.
func NewMCPHandler(m *jazmem.Memory) http.Handler {
	return mcpapi.NewHTTPHandler(m)
}

// NewAPIHandler serves jazmem's full HTTP API (/search, /page, /doctor,
// /reindex, /dream, /link-hygiene). jaz mounts it under /jazmem so the CLI
// can drive a running server instead of opening the database directly.
func NewAPIHandler(m *jazmem.Memory) http.Handler {
	return httpapi.New(m)
}
