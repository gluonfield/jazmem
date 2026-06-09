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

type Service struct {
	Memory *jazmem.Memory
}

func New(memory *jazmem.Memory) *mcp.Server {
	service := &Service{Memory: memory}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "jazmem",
		Title:   "Jazmem Memory",
		Version: serverVersion,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_search",
		Title:       "Search jazmem",
		Description: "Search jazmem and synthesize an evidence-grounded answer with citations and gaps. Use this before answering from memory.",
	}, service.Search)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_get",
		Title:       "Get jazmem markdown",
		Description: "Read a markdown memory page by slug. Returns raw markdown, file path metadata, and not-found suggestions for close slug matches.",
	}, service.GetPage)

	return server
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"question or topic to answer from jazmem memory"`
}

func (s *Service) Search(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, jazmem.AgenticResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, jazmem.AgenticResponse{}, errors.New("query is required")
	}
	response, err := s.Memory.AgenticSearch(ctx, query, jazmem.AgenticOptions{})
	return nil, response, err
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
