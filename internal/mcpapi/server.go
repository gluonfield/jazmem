package mcpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/internal/memfs"
	"github.com/gluonfield/jazmem/pkg/jazmem"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverVersion = "0.1.0"

const (
	SearchToolName     = "memory_search"
	GetPageToolName    = "memory_get_page"
	RawSearchToolName  = "jazmem_search_raw"
	RawGetPageToolName = "jazmem_get_page"
)

var toolNames = []string{SearchToolName, GetPageToolName}
var rawToolNames = []string{RawSearchToolName, RawGetPageToolName}

type Memory interface {
	AgenticSearch(context.Context, string, jazmem.AgenticOptions) (jazmem.AgenticResponse, error)
	GetPage(context.Context, string) (jazmem.Page, error)
}

type RawMemory interface {
	Retrieve(context.Context, string, jazmem.SearchOptions) (jazmem.SearchResponse, error)
	GetPage(context.Context, string) (jazmem.Page, error)
}

type PageReader interface {
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
		Name:        SearchToolName,
		Title:       "Search jazmem",
		Description: "Search jazmem and synthesize an evidence-grounded answer with citations and gaps. Use this before answering from memory. Set deep=true to spend more retrieval compute when a first answer is thin.",
	}, service.Search)
	AddGetPageTool(server, memory)
}

func AddGetPageTool(server *mcp.Server, memory PageReader) {
	addPageTool(server, memory, GetPageToolName, "Get jazmem memory page")
}

func AddRawTools(server *mcp.Server, memory RawMemory) {
	service := &RawService{Memory: memory}
	mcp.AddTool(server, &mcp.Tool{
		Name:        RawSearchToolName,
		Title:       "Raw jazmem search",
		Description: "Raw ranked jazmem retrieval. Start here, use concrete names and nouns, try variants when thin, and set deep=true only when linked context is likely to matter.",
	}, service.RawSearch)
	addPageTool(server, memory, RawGetPageToolName, "Get raw jazmem memory page")
}

func addPageTool(server *mcp.Server, memory PageReader, name, title string) {
	service := &PageService{Memory: memory}
	mcp.AddTool(server, &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: "Read one jazmem memory markdown page by path, such as people/alice or projects/jazmem. Returns raw markdown as text content plus compact metadata, links/backlinks, and not-found suggestions.",
	}, service.GetPage)
}

func RemoveTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(toolNames...)
	}
}

func RemoveGetPageTool(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(GetPageToolName)
	}
}

func RemoveRawTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(rawToolNames...)
	}
}

func ToolNames() []string {
	return append([]string(nil), toolNames...)
}

func RawToolNames() []string {
	return append([]string(nil), rawToolNames...)
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

type RawService struct {
	Memory RawMemory
}

type RawSearchInput struct {
	Query string `json:"query" jsonschema:"raw search query; use concrete names, entities, and phrases"`
	Limit int    `json:"limit,omitempty" jsonschema:"page limit, default 10, max 50"`
	Deep  bool   `json:"deep,omitempty" jsonschema:"wider retrieval with linked-page expansion"`
}

func (s *RawService) RawSearch(ctx context.Context, _ *mcp.CallToolRequest, input RawSearchInput) (*mcp.CallToolResult, jazmem.SearchResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, jazmem.SearchResponse{}, errors.New("query is required")
	}
	response, err := s.Memory.Retrieve(ctx, query, jazmem.SearchOptions{Limit: input.Limit, Deep: input.Deep})
	return nil, response, err
}

type PageService struct {
	Memory PageReader
}

type PageInput struct {
	Path string `json:"path" jsonschema:"memory page path, for example people/alice or projects/jazmem"`
}

type PageOutput struct {
	Found       bool                    `json:"found"`
	Error       string                  `json:"error,omitempty"`
	Suggestions []jazmem.SlugSuggestion `json:"suggestions,omitempty"`
	Path        string                  `json:"path,omitempty"`
	Aliases     []string                `json:"aliases,omitempty"`
	ModifiedAt  string                  `json:"modified_at,omitempty"`
	Links       []jazmem.LinkRef        `json:"links,omitempty"`
	Backlinks   []jazmem.LinkRef        `json:"backlinks,omitempty"`
}

func (s *PageService) GetPage(ctx context.Context, _ *mcp.CallToolRequest, input PageInput) (*mcp.CallToolResult, PageOutput, error) {
	path := cleanPagePath(input.Path)
	if path == "" {
		return nil, PageOutput{}, errors.New("path is required")
	}
	page, err := s.Memory.GetPage(ctx, path)
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
		Path:       page.Slug,
		Aliases:    page.Aliases,
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

func cleanPagePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "memory/")
	return memfs.CleanSlug(path)
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
