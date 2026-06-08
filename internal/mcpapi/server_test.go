package mcpapi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jazmem/pkg/jazmem"
)

func TestServerTools(t *testing.T) {
	mem := testMemory(t)
	defer mem.Close()

	if err := os.WriteFile(
		filepath.Join(mem.Root(), "people", "alice-bentick.md"),
		[]byte("---\ntitle: Alice Bentick\naliases: [Alice]\n---\n\n# Alice Bentick\n\nAlice works on jazmem MCP testing.\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), jazmem.ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	session := connectClient(t, New(mem))
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if len(names) != 2 || !names["jazmem_search"] || !names["jazmem_get"] {
		t.Fatalf("unexpected registered tools %#v", names)
	}

	searchCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "jazmem_search",
		Arguments: map[string]any{"query": "Alice jazmem MCP", "limit": 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if searchCall.IsError {
		t.Fatalf("search tool error: %#v", searchCall.Content)
	}
	search := decodeStructured[jazmem.SearchResponse](t, searchCall)
	if len(search.Results) == 0 || search.Results[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected search response %#v", search)
	}

	pageCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "jazmem_get",
		Arguments: map[string]any{"slug": "people/alice-bentick"},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := decodeStructured[PageOutput](t, pageCall)
	if !page.Found || page.Path == "" || page.Slug != "people/alice-bentick" || !strings.Contains(page.Raw, "Alice works on jazmem") {
		t.Fatalf("unexpected page response %#v", page)
	}
	if len(pageCall.Content) == 0 {
		t.Fatal("expected raw markdown text content")
	}
	text, ok := pageCall.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "Alice works on jazmem") {
		t.Fatalf("unexpected text content %#v", pageCall.Content)
	}

	missingCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "jazmem_get",
		Arguments: map[string]any{"slug": "people/alice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	missing := decodeStructured[PageOutput](t, missingCall)
	if missing.Found || missing.Error != "not found: people/alice" || len(missing.Suggestions) == 0 || missing.Suggestions[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected missing response %#v", missing)
	}
	missingText, ok := missingCall.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(missingText.Text, "not found: people/alice") || !strings.Contains(missingText.Text, "people/alice-bentick") {
		t.Fatalf("unexpected missing text content %#v", missingCall.Content)
	}
}

func testMemory(t *testing.T) *jazmem.Memory {
	t.Helper()
	mem, err := jazmem.Open(jazmem.Config{
		Root:   t.TempDir(),
		DBPath: filepath.Join(t.TempDir(), "index.sqlite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return mem
}

func connectClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "jazmem-test-client", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	return clientSession
}

func decodeStructured[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	var out T
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode structured content: %v\n%s", err, string(data))
	}
	return out
}
