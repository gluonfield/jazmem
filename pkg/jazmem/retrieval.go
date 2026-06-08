package jazmem

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jazmem/internal/search"
	sqlitestore "github.com/wins/jazmem/internal/store/sqlite"
)

func (m *Memory) Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
	limit := normalizeSearchLimit(opts.Limit)
	rows, err := m.search.Search(ctx, query, search.Options{Limit: chunkCandidateLimit(limit)})
	if err != nil {
		return nil, err
	}
	return mergeChunkResults(rows, limit), nil
}

func (m *Memory) Retrieve(ctx context.Context, query string, opts SearchOptions) (SearchResponse, error) {
	results, err := m.Search(ctx, query, opts)
	if err != nil {
		return SearchResponse{}, err
	}
	chunks := 0
	for _, result := range results {
		chunks += len(result.Matches)
	}
	limit := normalizeSearchLimit(opts.Limit)
	return SearchResponse{
		Query:   strings.TrimSpace(query),
		Limit:   limit,
		Results: results,
		Stats: SearchStats{
			Pages:  len(results),
			Chunks: chunks,
		},
	}, nil
}

func RenderSearchText(response SearchResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Query: %s\n", response.Query)
	fmt.Fprintf(&b, "Results: %d pages, %d matched chunks\n\n", response.Stats.Pages, response.Stats.Chunks)
	if len(response.Results) == 0 {
		b.WriteString("No matching memory chunks were found.\n")
		return b.String()
	}
	for i, result := range response.Results {
		fmt.Fprintf(&b, "[%d] %s (%s, %.8f)\n", i+1, result.Title, result.Slug, result.Score)
		for _, match := range result.Matches {
			if strings.TrimSpace(match.Snippet) == "" {
				continue
			}
			fmt.Fprintf(&b, "  chunk %d: %s\n", match.Chunk, strings.TrimSpace(match.Snippet))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func mergeChunkResults(rows []sqlitestore.SearchResult, limit int) []Result {
	results := make([]Result, 0, limit)
	bySlug := map[string]int{}
	for _, row := range rows {
		idx, ok := bySlug[row.Slug]
		if !ok {
			if len(results) >= limit {
				continue
			}
			idx = len(results)
			bySlug[row.Slug] = idx
			results = append(results, Result{
				Slug:  row.Slug,
				Title: row.Title,
				Score: row.Score,
			})
		}
		result := &results[idx]
		if len(result.Matches) == 0 || row.Score < result.Score {
			result.Score = row.Score
		}
		result.Matches = append(result.Matches, Match{
			Chunk:   row.ChunkIndex,
			Snippet: row.Snippet,
			Score:   row.Score,
		})
	}
	return results
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func chunkCandidateLimit(pageLimit int) int {
	limit := pageLimit * 4
	if limit < 10 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	return limit
}
