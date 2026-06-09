package mcpapi

import (
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jazmem/pkg/jazmem"
)

func NewHTTPHandler(memory *jazmem.Memory) http.Handler {
	server := New(memory)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		JSONResponse:   true,
		SessionTimeout: 30 * time.Minute,
	})
}
