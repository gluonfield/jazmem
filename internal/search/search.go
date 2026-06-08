package search

import (
	"context"
	"sort"

	sqlitestore "github.com/wins/jazmem/internal/store/sqlite"
)

type Service struct {
	Store *sqlitestore.Store
}

type Options struct {
	Limit int
}

type Response struct {
	Rows      []sqlitestore.SearchResult
	GraphHits int
}

func (s *Service) Search(ctx context.Context, query string, opts Options) (Response, error) {
	limit := normalizeLimit(opts.Limit)
	titleRows, err := s.Store.SearchTitleAlias(ctx, query, limit)
	if err != nil {
		return Response{}, err
	}
	bm25Rows, err := s.Store.Search(ctx, query, chunkCandidateLimit(limit))
	if err != nil {
		return Response{}, err
	}
	merged := mergeRows(titleRows, bm25Rows)
	seeds := topSlugs(merged, limit)
	graphRows, err := s.Store.LinkedPages(ctx, seeds, limit)
	if err != nil {
		return Response{}, err
	}
	graphHits := countNewSlugs(merged, graphRows)
	merged = mergeRows(merged, graphRows)
	if len(merged) > chunkCandidateLimit(limit) {
		merged = merged[:chunkCandidateLimit(limit)]
	}
	return Response{Rows: merged, GraphHits: graphHits}, nil
}

func mergeRows(groups ...[]sqlitestore.SearchResult) []sqlitestore.SearchResult {
	byMatch := map[string]sqlitestore.SearchResult{}
	for _, group := range groups {
		for _, row := range group {
			key := row.Slug + "\x00" + string(rune(row.ChunkIndex))
			current, ok := byMatch[key]
			if !ok || row.Score < current.Score {
				byMatch[key] = row
			}
		}
	}
	rows := make([]sqlitestore.SearchResult, 0, len(byMatch))
	for _, row := range byMatch {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(a, b int) bool {
		if rows[a].Score == rows[b].Score {
			if rows[a].Slug == rows[b].Slug {
				return rows[a].ChunkIndex < rows[b].ChunkIndex
			}
			return rows[a].Slug < rows[b].Slug
		}
		return rows[a].Score < rows[b].Score
	})
	return rows
}

func topSlugs(rows []sqlitestore.SearchResult, limit int) []string {
	seen := map[string]bool{}
	slugs := make([]string, 0, limit)
	for _, row := range rows {
		if seen[row.Slug] {
			continue
		}
		seen[row.Slug] = true
		slugs = append(slugs, row.Slug)
		if len(slugs) >= limit {
			break
		}
	}
	return slugs
}

func countNewSlugs(existing, added []sqlitestore.SearchResult) int {
	seen := map[string]bool{}
	for _, row := range existing {
		seen[row.Slug] = true
	}
	count := 0
	for _, row := range added {
		if seen[row.Slug] {
			continue
		}
		seen[row.Slug] = true
		count++
	}
	return count
}

func normalizeLimit(limit int) int {
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
