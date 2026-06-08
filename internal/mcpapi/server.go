package mcpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jazmem/pkg/jazmem"
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
		Description: "Run deterministic jazmem retrieval over markdown memory. Use this before answering from memory or deciding which pages to read/edit.",
	}, service.Search)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_answer",
		Title:       "Answer from jazmem",
		Description: "Synthesize an answer from retrieved jazmem evidence using OpenRouter. Requires OPENROUTER_API_KEY and ignores raw search limit tuning.",
	}, service.Answer)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_get_page",
		Title:       "Get jazmem page",
		Description: "Read a markdown memory page by slug. Returns not-found suggestions instead of failing when the slug is close but not exact.",
	}, service.GetPage)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_file",
		Title:       "Resolve jazmem file",
		Description: "Resolve a page slug to the canonical markdown file path for direct agent editing. Returns similar slugs when not found.",
	}, service.File)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_index",
		Title:       "Reindex jazmem",
		Description: "Rebuild the SQLite search/link index from markdown after files are edited.",
	}, service.Index)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_doctor",
		Title:       "Inspect jazmem",
		Description: "Return jazmem root, SQLite path, and index counts.",
	}, service.Doctor)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_dream",
		Title:       "Run jazmem dream",
		Description: "Run OpenRouter-backed consolidation. Writes dream run/review markdown and promotes only validated cited bullets.",
	}, service.Dream)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_link_hygiene",
		Title:       "Run jazmem link hygiene",
		Description: "Generate relationship/link review proposals from markdown and indexed mentions.",
	}, service.LinkHygiene)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jazmem_checkpoint",
		Title:       "Checkpoint jazmem",
		Description: "Commit meaningful markdown memory progress after editing, indexing, and verifying retrieval.",
	}, service.Checkpoint)

	return server
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"memory query to search for"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum returned pages for raw retrieval; defaults to 10"`
}

func (s *Service) Search(ctx context.Context, _ *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, jazmem.SearchResponse, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return nil, jazmem.SearchResponse{}, errors.New("query is required")
	}
	limit, err := normalizeOptionalLimit(input.Limit)
	if err != nil {
		return nil, jazmem.SearchResponse{}, err
	}
	response, err := s.Memory.Retrieve(ctx, query, jazmem.SearchOptions{Limit: limit})
	return nil, response, err
}

type AnswerInput struct {
	Query string `json:"query" jsonschema:"question to answer from jazmem evidence"`
}

func (s *Service) Answer(ctx context.Context, _ *mcp.CallToolRequest, input AnswerInput) (*mcp.CallToolResult, jazmem.AgenticResponse, error) {
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
	Page        *PagePayload            `json:"page,omitempty"`
}

type PagePayload struct {
	Slug        string         `json:"slug"`
	Path        string         `json:"path"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Aliases     []string       `json:"aliases"`
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
	Raw         string         `json:"raw"`
	ModifiedAt  string         `json:"modified_at"`
}

func (s *Service) GetPage(ctx context.Context, _ *mcp.CallToolRequest, input PageInput) (*mcp.CallToolResult, PageOutput, error) {
	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		return nil, PageOutput{}, errors.New("slug is required")
	}
	page, err := s.Memory.GetPage(ctx, slug)
	if err != nil {
		return nil, notFoundPageOutput(err), nil
	}
	return nil, PageOutput{Found: true, Page: pagePayload(page)}, nil
}

type FileOutput struct {
	Found       bool                    `json:"found"`
	Error       string                  `json:"error,omitempty"`
	Slug        string                  `json:"slug,omitempty"`
	Title       string                  `json:"title,omitempty"`
	Path        string                  `json:"path,omitempty"`
	Suggestions []jazmem.SlugSuggestion `json:"suggestions,omitempty"`
}

func (s *Service) File(ctx context.Context, _ *mcp.CallToolRequest, input PageInput) (*mcp.CallToolResult, FileOutput, error) {
	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		return nil, FileOutput{}, errors.New("slug is required")
	}
	page, err := s.Memory.GetPage(ctx, slug)
	if err != nil {
		return nil, notFoundFileOutput(err), nil
	}
	return nil, FileOutput{
		Found: true,
		Slug:  page.Slug,
		Title: page.Title,
		Path:  page.Path,
	}, nil
}

type EmptyInput struct{}

func (s *Service) Index(ctx context.Context, _ *mcp.CallToolRequest, _ EmptyInput) (*mcp.CallToolResult, jazmem.Report, error) {
	report, err := s.Memory.Reindex(ctx, jazmem.ReindexOptions{})
	return nil, report, err
}

func (s *Service) Doctor(ctx context.Context, _ *mcp.CallToolRequest, _ EmptyInput) (*mcp.CallToolResult, jazmem.DoctorReport, error) {
	report, err := s.Memory.Doctor(ctx)
	return nil, report, err
}

type DreamInput struct {
	Date string `json:"date,omitempty" jsonschema:"optional local date in YYYY-MM-DD format"`
}

func (s *Service) Dream(ctx context.Context, _ *mcp.CallToolRequest, input DreamInput) (*mcp.CallToolResult, jazmem.DreamReport, error) {
	var date time.Time
	if strings.TrimSpace(input.Date) != "" {
		parsed, err := time.Parse("2006-01-02", strings.TrimSpace(input.Date))
		if err != nil {
			return nil, jazmem.DreamReport{}, fmt.Errorf("parse date: %w", err)
		}
		date = parsed
	}
	report, err := s.Memory.Dream(ctx, jazmem.DreamOptions{Date: date})
	return nil, report, err
}

func (s *Service) LinkHygiene(ctx context.Context, _ *mcp.CallToolRequest, _ EmptyInput) (*mcp.CallToolResult, jazmem.LinkHygieneReport, error) {
	report, err := s.Memory.LinkHygiene(ctx)
	return nil, report, err
}

type CheckpointInput struct {
	Message string `json:"message" jsonschema:"git commit message for markdown memory progress"`
}

func (s *Service) Checkpoint(ctx context.Context, _ *mcp.CallToolRequest, input CheckpointInput) (*mcp.CallToolResult, jazmem.CheckpointReport, error) {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return nil, jazmem.CheckpointReport{}, errors.New("message is required")
	}
	report, err := s.Memory.Checkpoint(ctx, message)
	return nil, report, err
}

func normalizeOptionalLimit(limit int) (int, error) {
	if limit == 0 {
		return 10, nil
	}
	if limit < 0 {
		return 0, errors.New("limit must be positive")
	}
	return limit, nil
}

func pagePayload(page jazmem.Page) *PagePayload {
	return &PagePayload{
		Slug:        page.Slug,
		Path:        page.Path,
		Type:        page.Type,
		Title:       page.Title,
		Aliases:     page.Aliases,
		Frontmatter: page.Frontmatter,
		Body:        page.Body,
		Raw:         page.Raw,
		ModifiedAt:  page.ModifiedAt.Format(time.RFC3339Nano),
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

func notFoundFileOutput(err error) FileOutput {
	var notFound *jazmem.NotFoundError
	if errors.As(err, &notFound) {
		return FileOutput{
			Found:       false,
			Error:       notFound.Error(),
			Suggestions: notFound.Suggestions,
		}
	}
	return FileOutput{Found: false, Error: err.Error()}
}
