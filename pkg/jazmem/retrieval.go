package jazmem

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jazmem/internal/search"
)

func (m *Memory) Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
	rows, err := m.search.Search(ctx, query, search.Options{Limit: opts.Limit})
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(rows))
	for _, row := range rows {
		results = append(results, Result{
			Slug:       row.Slug,
			Title:      row.Title,
			ChunkIndex: row.ChunkIndex,
			Snippet:    row.Snippet,
			Score:      row.Score,
		})
	}
	return results, nil
}

func (m *Memory) Retrieve(ctx context.Context, query string, opts SearchOptions) (RetrievalContext, error) {
	results, err := m.Search(ctx, query, opts)
	if err != nil {
		return RetrievalContext{}, err
	}
	citations := make([]Citation, 0, len(results))
	warnings := make([]string, 0)
	pages := map[string]bool{}
	for _, result := range results {
		path := ""
		if page, err := m.GetPage(ctx, result.Slug); err == nil {
			path = page.Path
		} else {
			warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", result.Slug, err))
		}
		pages[result.Slug] = true
		citations = append(citations, Citation{
			Slug:       result.Slug,
			Title:      result.Title,
			Path:       path,
			ChunkIndex: result.ChunkIndex,
			Snippet:    result.Snippet,
			Score:      result.Score,
		})
	}
	contextText := renderRetrievalContext(query, citations)
	return RetrievalContext{
		Query:          query,
		Context:        contextText,
		Citations:      citations,
		PagesGathered:  len(pages),
		ChunksGathered: len(results),
		Warnings:       warnings,
		Diagnostics: RetrievalDiagnostics{
			PagesFromBM25:  len(pages),
			ChunksFromBM25: len(results),
			Mode:           "bm25",
		},
		Results: results,
	}, nil
}

func (m *Memory) SearchContext(ctx context.Context, query string, opts SearchOptions) (SearchContext, error) {
	return m.Retrieve(ctx, query, opts)
}

func renderRetrievalContext(query string, citations []Citation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Retrieved Memory Context\n\nQuery: %s\n\n", query)
	if len(citations) == 0 {
		b.WriteString("No matching memory chunks were found.\n")
		return b.String()
	}
	for i, citation := range citations {
		fmt.Fprintf(&b, "## [%d] %s\n\n", i+1, citation.Title)
		fmt.Fprintf(&b, "Slug: %s\n", citation.Slug)
		if citation.Path != "" {
			fmt.Fprintf(&b, "File: %s\n", citation.Path)
		}
		fmt.Fprintf(&b, "Chunk: %d\n", citation.ChunkIndex)
		fmt.Fprintf(&b, "Score: %.8f\n\n", citation.Score)
		if strings.TrimSpace(citation.Snippet) != "" {
			b.WriteString(strings.TrimSpace(citation.Snippet))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}
