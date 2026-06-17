package mcpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerTools(t *testing.T) {
	llm := fakeProvider(t, `{"answer":"Alice works on jazmem MCP testing.","citation_ids":[1],"gaps":[],"warnings":[]}`)
	defer llm.Close()
	mem := testMemory(t, jazmem.Config{
		APIKey:           "test-key",
		ProviderEndpoint: llm.URL,
		Model:            "test-model",
	})
	defer func() { _ = mem.Close() }()

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
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if len(names) != 2 || !names[SearchToolName] || !names[GetPageToolName] {
		t.Fatalf("unexpected registered tools %#v", names)
	}

	searchCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      SearchToolName,
		Arguments: map[string]any{"query": "Alice jazmem MCP"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if searchCall.IsError {
		t.Fatalf("search tool error: %#v", searchCall.Content)
	}
	search := decodeStructured[jazmem.AgenticResponse](t, searchCall)
	if !strings.Contains(search.Answer, "Alice works") || search.ModelUsed != "test-model" || len(search.Citations) == 0 || search.Citations[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected search response %#v", search)
	}

	pageCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      GetPageToolName,
		Arguments: map[string]any{"path": "people/alice-bentick"},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := decodeStructured[PageOutput](t, pageCall)
	if !page.Found || page.Path != "people/alice-bentick" {
		t.Fatalf("unexpected page response %#v", page)
	}
	assertNoStructuredKeys(t, pageCall, "raw", "slug", "title", "type")
	if len(pageCall.Content) == 0 {
		t.Fatal("expected raw markdown text content")
	}
	text, ok := pageCall.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "Alice works on jazmem") {
		t.Fatalf("unexpected text content %#v", pageCall.Content)
	}

	missingCall, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      GetPageToolName,
		Arguments: map[string]any{"path": "people/alice"},
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

	rawServer := mcp.NewServer(&mcp.Implementation{Name: "raw-test", Version: "0.0.1"}, nil)
	AddRawTools(rawServer, mem)
	rawSession := connectClient(t, rawServer)
	defer func() { _ = rawSession.Close() }()
	rawTools, err := rawSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rawNames := map[string]bool{}
	for _, tool := range rawTools.Tools {
		rawNames[tool.Name] = true
	}
	if len(rawNames) != 2 || !rawNames[RawSearchToolName] || !rawNames[RawGetPageToolName] {
		t.Fatalf("unexpected raw tools %#v", rawNames)
	}
	rawSearchCall, err := rawSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      RawSearchToolName,
		Arguments: map[string]any{"query": "Alice jazmem MCP"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rawSearch := decodeStructured[jazmem.SearchResponse](t, rawSearchCall)
	if len(rawSearch.Results) == 0 || rawSearch.Results[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected raw search response %#v", rawSearch)
	}
	rawPageCall, err := rawSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      RawGetPageToolName,
		Arguments: map[string]any{"path": "memory/people/alice-bentick.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rawPage := decodeStructured[PageOutput](t, rawPageCall)
	if !rawPage.Found || rawPage.Path != "people/alice-bentick" {
		t.Fatalf("unexpected raw page response %#v", rawPage)
	}
}

func testMemory(t *testing.T, cfg jazmem.Config) *jazmem.Memory {
	t.Helper()
	cfg.Root = t.TempDir()
	cfg.DBPath = filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := jazmem.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return mem
}

func fakeProvider(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected provider path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": content}},
			},
		})
	}))
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

func assertNoStructuredKeys(t *testing.T, result *mcp.CallToolResult, keys ...string) {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode structured content: %v\n%s", err, string(data))
	}
	for _, key := range keys {
		if _, ok := payload[key]; ok {
			t.Fatalf("structured content unexpectedly contains %q: %s", key, string(data))
		}
	}
}
