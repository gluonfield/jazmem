package jazmemhttp

import (
	"context"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPToolRegistration(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	AddMCPTools(server, fakeMCPMemory{})

	session := connectTestClient(t, server)
	defer func() { _ = session.Close() }()

	assertTools(t, session, "memory_search", "memory_get_page")

	RemoveMCPTools(server)

	assertTools(t, session)

	AddMCPGetPageTool(server, fakeMCPMemory{})
	assertTools(t, session, "memory_get_page")
	RemoveMCPGetPageTool(server)
	assertTools(t, session)

	AddRawMCPTools(server, fakeMCPMemory{})
	assertTools(t, session, "jazmem_search_raw", "jazmem_get_page")
	RemoveRawMCPTools(server)
	assertTools(t, session)
}

type fakeMCPMemory struct{}

func (fakeMCPMemory) AgenticSearch(context.Context, string, jazmem.AgenticOptions) (jazmem.AgenticResponse, error) {
	return jazmem.AgenticResponse{}, nil
}

func (fakeMCPMemory) Retrieve(context.Context, string, jazmem.SearchOptions) (jazmem.SearchResponse, error) {
	return jazmem.SearchResponse{}, nil
}

func (fakeMCPMemory) GetPage(context.Context, string) (jazmem.Page, error) {
	return jazmem.Page{}, nil
}

func connectTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func assertTools(t *testing.T, session *mcp.ClientSession, want ...string) {
	t.Helper()
	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if len(names) != len(want) {
		t.Fatalf("tools = %#v, want %v", names, want)
	}
	for _, name := range want {
		if !names[name] {
			t.Fatalf("tools = %#v, missing %s", names, name)
		}
	}
}
