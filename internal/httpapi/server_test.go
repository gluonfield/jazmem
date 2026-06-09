package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSearchEndpoint(t *testing.T) {
	llm := fakeProvider(t, `{"answer":"jazmem search is being tested by Alice and Riley.","citation_ids":[1],"gaps":[],"warnings":[]}`)
	defer llm.Close()
	mem, err := jazmem.Open(jazmem.Config{
		Root:             t.TempDir(),
		DBPath:           filepath.Join(t.TempDir(), "index.sqlite"),
		APIKey:           "test-key",
		ProviderEndpoint: llm.URL,
		Model:            "test-model",
		Now: func() time.Time {
			return time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	handler := New(mem)
	path := filepath.Join(mem.Root(), "inbox", "search-note.md")
	if err := os.WriteFile(path, []byte("---\ntitle: Search note\ntype: inbox\n---\n\n# Search note\n\nAlice and Riley are testing jazmem search.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(t.Context(), jazmem.ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	searchReq := httptest.NewRequest(http.MethodGet, "/search?q=jazmem&limit=3", nil)
	searchResp := httptest.NewRecorder()
	handler.ServeHTTP(searchResp, searchReq)
	if searchResp.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", searchResp.Code, searchResp.Body.String())
	}
	var payload jazmem.SearchResponse
	if err := json.Unmarshal(searchResp.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) != 1 || payload.Results[0].Title != "Search note" {
		t.Fatalf("unexpected results %#v", payload.Results)
	}
	if payload.Stats.Pages != 1 || payload.Stats.Chunks != 1 {
		t.Fatalf("unexpected search envelope %#v", payload)
	}

	agenticReq := httptest.NewRequest(http.MethodGet, "/search?q=jazmem&agentic=1", nil)
	agenticResp := httptest.NewRecorder()
	handler.ServeHTTP(agenticResp, agenticReq)
	if agenticResp.Code != http.StatusOK {
		t.Fatalf("agentic search status = %d body=%s", agenticResp.Code, agenticResp.Body.String())
	}
	var agentic jazmem.AgenticResponse
	if err := json.Unmarshal(agenticResp.Body.Bytes(), &agentic); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(agentic.Answer, "jazmem") || len(agentic.Citations) == 0 || agentic.Citations[0].Slug != "inbox/search-note" {
		t.Fatalf("unexpected agentic payload %#v", agentic)
	}
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

func TestFileEndpointSuggestsSimilarSlugs(t *testing.T) {
	mem, err := jazmem.Open(jazmem.Config{
		Root:   t.TempDir(),
		DBPath: filepath.Join(t.TempDir(), "index.sqlite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	path := filepath.Join(mem.Root(), "people", "alice-bentick.md")
	if err := os.WriteFile(path, []byte("---\ntitle: Alice Bentick\naliases: [Alice]\n---\n\n# Alice Bentick\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/file/people/alice", nil)
	resp := httptest.NewRecorder()
	New(mem).ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Error       string                  `json:"error"`
		Suggestions []jazmem.SlugSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "not found: people/alice" || len(payload.Suggestions) == 0 || payload.Suggestions[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected payload %#v", payload)
	}
}

func TestMCPEndpointUsesServerMemory(t *testing.T) {
	llm := fakeProvider(t, `{"answer":"Alice works on jazmem HTTP MCP.","citation_ids":[1],"gaps":[],"warnings":[]}`)
	defer llm.Close()
	mem, err := jazmem.Open(jazmem.Config{
		Root:             t.TempDir(),
		DBPath:           filepath.Join(t.TempDir(), "index.sqlite"),
		APIKey:           "test-key",
		ProviderEndpoint: llm.URL,
		Model:            "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()
	if err := os.WriteFile(
		filepath.Join(mem.Root(), "people", "alice-http-mcp.md"),
		[]byte("---\ntitle: Alice HTTP MCP\naliases: [Alice]\n---\n\n# Alice HTTP MCP\n\nAlice works on jazmem HTTP MCP.\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(t.Context(), jazmem.ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	httpServer := httptest.NewServer(New(mem))
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "jazmem-http-test", Version: "0.0.1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if !names["jazmem_search"] || !names["jazmem_get"] {
		t.Fatalf("unexpected MCP tools %#v", names)
	}

	searchCall, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "jazmem_search",
		Arguments: map[string]any{"query": "Alice HTTP MCP"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var search jazmem.AgenticResponse
	data, err := json.Marshal(searchCall.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &search); err != nil {
		t.Fatalf("decode MCP search: %v\n%s", err, string(data))
	}
	if !strings.Contains(search.Answer, "Alice works") || len(search.Citations) == 0 || search.Citations[0].Slug != "people/alice-http-mcp" {
		t.Fatalf("unexpected MCP search response %#v", search)
	}

	pageCall, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "jazmem_get",
		Arguments: map[string]any{"slug": "people/alice-http-mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pageCall.Content) == 0 {
		t.Fatal("expected markdown content")
	}
	text, ok := pageCall.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "Alice works on jazmem HTTP MCP") {
		t.Fatalf("unexpected MCP page content %#v", pageCall.Content)
	}
}
