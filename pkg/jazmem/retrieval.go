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
			Slug:    row.Slug,
			Title:   row.Title,
			Chunk:   row.ChunkIndex,
			Snippet: row.Snippet,
			Score:   row.Score,
		})
	}
	return results, nil
}

func (m *Memory) Retrieve(ctx context.Context, query string, opts SearchOptions) (SearchResponse, error) {
	results, err := m.Search(ctx, query, opts)
	if err != nil {
		return SearchResponse{}, err
	}
	pages := map[string]bool{}
	for _, result := range results {
		pages[result.Slug] = true
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	return SearchResponse{
		Query:   strings.TrimSpace(query),
		Limit:   limit,
		Results: results,
		Stats: SearchStats{
			Pages:  len(pages),
			Chunks: len(results),
			Mode:   "bm25",
		},
	}, nil
}

func RenderSearchText(response SearchResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Query: %s\n", response.Query)
	fmt.Fprintf(&b, "Results: %d chunks across %d pages (%s)\n\n", response.Stats.Chunks, response.Stats.Pages, response.Stats.Mode)
	if len(response.Results) == 0 {
		b.WriteString("No matching memory chunks were found.\n")
		return b.String()
	}
	for i, result := range response.Results {
		fmt.Fprintf(&b, "[%d] %s (%s#%d, %.8f)\n", i+1, result.Title, result.Slug, result.Chunk, result.Score)
		if strings.TrimSpace(result.Snippet) != "" {
			b.WriteString(strings.TrimSpace(result.Snippet))
			b.WriteString("\n\n")
		}
	}
	return b.String()
}
