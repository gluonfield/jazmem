package mcpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverVersion = "0.1.0"

var toolNames = []string{"memory_search", "memory_search_raw", "memory_get"}

type Memory interface {
	AgenticSearch(context.Context, string, jazmem.AgenticOptions) (jazmem.AgenticResponse, error)
	Retrieve(context.Context, string, jazmem.SearchOptions) (jazmem.SearchResponse, error)
	GetPage(context.Context, string) (jazmem.Page, error)
}

type Service struct {
	Memory Memory
}

func New(memory Memory) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "jazmem",
		Title:   "Jazmem Memory",
		Version: serverVersion,
	}, nil)
	AddTools(server, memory)
	return server
}

func AddTools(server *mcp.Server, memory Memory) {
	service := &Service{Memory: memory}
	mcp.AddTool(server, &mcp.Tool{
		Name:        toolNames[0],
		Title:       "Search jazmem",
		Description: "Search jazmem and synthesize an evidence-grounded answer with citations and gaps. Use this before answering from memory. Set deep=true to spend more retrieval compute when a first answer is thin.",
	}, service.Search)
	mcp.AddTool(server, &mcp.Tool{
		Name:        toolNames[1],
		Title:       "Raw search jazmem",
		Description: "Deterministic ranked retrieval with no LLM call. Returns pages with matched chunk snippets and scores. Use it to pick pages to read or edit, or to drive your own deeper search loop.",
	}, service.SearchRaw)
	mcp.AddTool(server, &mcp.Tool{
		Name:        toolNames[2],
		Title:       "Get jazmem markdown",
		Description: "Read a markdown memory page by slug. Returns raw markdown, file path metadata, and not-found suggestions for close slug matches.",
	}, service.GetPage)
}

func RemoveTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(toolNames...)
	}
}

func ToolNames() []string {
	return append([]string(nil), toolNames...)
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"question or topic to answer from jazmem memory"`
	Deep  bool   `json:"deep,omitempty" jsonschema:"spend more retrieval compute: wider candidate pool, two-hop link expansion, and a gap-driven second retrieval round"`
}

func (s *Service) Search(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, jazmem.AgenticResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, jazmem.AgenticResponse{}, errors.New("query is required")
	}
	response, err := s.Memory.AgenticSearch(ctx, query, jazmem.AgenticOptions{Deep: input.Deep})
	return nil, response, err
}

type RawSearchInput struct {
	Query string `json:"query" jsonschema:"search terms; names and concrete nouns work best"`
	Limit int    `json:"limit,omitempty" jsonschema:"max pages to return, 1-50, default 10"`
	Deep  bool   `json:"deep,omitempty" jsonschema:"wider candidate pool and two-hop link expansion"`
}

func (s *Service) SearchRaw(ctx context.Context, _ *mcp.CallToolRequest, input RawSearchInput) (*mcp.CallToolResult, jazmem.SearchResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, jazmem.SearchResponse{}, errors.New("query is required")
	}
	response, err := s.Memory.Retrieve(ctx, query, jazmem.SearchOptions{Limit: input.Limit, Deep: input.Deep})
	if err != nil {
		return nil, jazmem.SearchResponse{}, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: jazmem.RenderSearchText(response)}},
	}, response, nil
}

type PageInput struct {
	Slug string `json:"slug" jsonschema:"markdown page slug, for example people/alice or projects/jazmem"`
}

type PageOutput struct {
	Found       bool                    `json:"found"`
	Error       string                  `json:"error,omitempty"`
	Suggestions []jazmem.SlugSuggestion `json:"suggestions,omitempty"`
	Slug        string                  `json:"slug,omitempty"`
	Path        string                  `json:"path,omitempty"`
	Type        string                  `json:"type,omitempty"`
	Title       string                  `json:"title,omitempty"`
	Aliases     []string                `json:"aliases,omitempty"`
	Raw         string                  `json:"raw,omitempty"`
	ModifiedAt  string                  `json:"modified_at,omitempty"`
	Links       []jazmem.LinkRef        `json:"links,omitempty"`
	Backlinks   []jazmem.LinkRef        `json:"backlinks,omitempty"`
}

func (s *Service) GetPage(ctx context.Context, _ *mcp.CallToolRequest, input PageInput) (*mcp.CallToolResult, PageOutput, error) {
	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		return nil, PageOutput{}, errors.New("slug is required")
	}
	page, err := s.Memory.GetPage(ctx, slug)
	if err != nil {
		output := notFoundPageOutput(err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: notFoundText(output)}},
		}, output, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: page.Raw}},
	}, pageOutput(page), nil
}

func pageOutput(page jazmem.Page) PageOutput {
	return PageOutput{
		Found:      true,
		Slug:       page.Slug,
		Path:       page.Path,
		Type:       page.Type,
		Title:      page.Title,
		Aliases:    page.Aliases,
		Raw:        page.Raw,
		ModifiedAt: page.ModifiedAt.Format(time.RFC3339Nano),
		Links:      page.Links,
		Backlinks:  page.Backlinks,
	}
}

func notFoundPageOutput(err error) PageOutput {
	var notFound *jazmem.NotFoundError
	if errors.As(err, &notFound) {
		return PageOutput{
			Found:       false,
			Error:       notFound.Error(),
			Suggestions: notFound.Suggestions,
		}
	}
	return PageOutput{Found: false, Error: err.Error()}
}

func notFoundText(output PageOutput) string {
	if output.Error == "" {
		return "not found"
	}
	var b strings.Builder
	b.WriteString(output.Error)
	if len(output.Suggestions) == 0 {
		return b.String()
	}
	b.WriteString("\nsuggestions:")
	for _, suggestion := range output.Suggestions {
		if suggestion.Title == "" {
			fmt.Fprintf(&b, "\n- %s", suggestion.Slug)
			continue
		}
		fmt.Fprintf(&b, "\n- %s (%s)", suggestion.Slug, suggestion.Title)
	}
	return b.String()
}
