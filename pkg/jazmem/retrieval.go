package jazmem

import (
	"context"

	"github.com/gluonfield/jazmem/internal/search"
	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
)

const (
	agenticRetrievalLimit     = 12
	agenticDeepRetrievalLimit = 30
	agenticEvidenceLimit      = 20
	agenticDeepEvidenceLimit  = 48
	agenticFollowupQueryLimit = 3
	agenticFollowupPageLimit  = 8
)

func (m *Memory) Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
	response, err := m.retrieve(ctx, query, opts.Limit, opts.Deep)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (m *Memory) Retrieve(ctx context.Context, query string, opts SearchOptions) (SearchResponse, error) {
	return m.retrieve(ctx, query, opts.Limit, opts.Deep)
}

func (m *Memory) AgenticSearch(ctx context.Context, query string, opts AgenticOptions) (AgenticResponse, error) {
	limit, evidenceCap := agenticRetrievalLimit, agenticEvidenceLimit
	if opts.Deep {
		limit, evidenceCap = agenticDeepRetrievalLimit, agenticDeepEvidenceLimit
	}
	response, err := m.retrieve(ctx, query, limit, opts.Deep)
	if err != nil {
		return AgenticResponse{}, err
	}
	return m.synthesizeAgentic(ctx, query, response, opts.Deep, evidenceCap)
}

func (m *Memory) retrieve(ctx context.Context, query string, rawLimit int, deep bool) (SearchResponse, error) {
	limit := normalizeSearchLimit(rawLimit)
	candidates, err := m.search.Search(ctx, query, search.Options{Limit: limit, Deep: deep})
	if err != nil {
		return SearchResponse{}, err
	}
	results := mergeChunkResults(candidates.Rows, limit)
	if err := m.attachPageMeta(ctx, results); err != nil {
		return SearchResponse{}, err
	}
	chunks := 0
	for _, result := range results {
		chunks += len(result.Matches)
	}
	return SearchResponse{
		Results: results,
		Stats: SearchStats{
			Pages:     len(results),
			Chunks:    chunks,
			GraphHits: candidates.GraphHits,
		},
	}, nil
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
				Via:   row.Via,
			})
		}
		result := &results[idx]
		if len(result.Matches) == 0 || row.Score < result.Score {
			result.Score = row.Score
			result.Via = row.Via
		}
		result.Matches = append(result.Matches, Match{
			Chunk:   row.ChunkIndex,
			Snippet: row.Snippet,
			Score:   row.Score,
		})
	}
	return results
}

func (m *Memory) attachPageMeta(ctx context.Context, results []Result) error {
	if len(results) == 0 {
		return nil
	}
	slugs := make([]string, 0, len(results))
	for _, result := range results {
		slugs = append(slugs, result.Slug)
	}
	metas, err := m.store.PageMetas(ctx, slugs)
	if err != nil {
		return err
	}
	for i := range results {
		if meta, ok := metas[results[i].Slug]; ok {
			results[i].ModifiedAt = meta.ModifiedAt
		}
	}
	return nil
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
