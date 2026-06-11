package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/gluonfield/jazmem/internal/store/sqlite/generated/searchdb"
)

func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	tokens := ftsTokens(query)
	if match := ftsQueryAll(tokens); match != "" {
		results, err := s.searchFTS(ctx, match, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if err != nil && !isFTSQuerySyntaxError(err) {
			return nil, err
		}
	}
	if match := ftsQueryAny(tokens); match != "" {
		results, err := s.searchFTS(ctx, match, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		if err != nil && !isFTSQuerySyntaxError(err) {
			return nil, err
		}
	}
	return s.searchLike(ctx, query, limit)
}

func (s *Store) SearchTitleAlias(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	terms := titleAliasLookupTerms(query)
	if len(terms) == 0 {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	bySlug := map[string]SearchResult{}
	for _, term := range terms {
		rows, err := s.searchTitleAliasTerm(ctx, term, limit)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			current, ok := bySlug[row.Slug]
			if !ok || row.Score < current.Score {
				bySlug[row.Slug] = row
			}
		}
	}
	results := make([]SearchResult, 0, len(bySlug))
	for _, row := range bySlug {
		results = append(results, row)
	}
	sortSearchResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *Store) LinkedPages(ctx context.Context, seeds []string, limit int) ([]SearchResult, error) {
	seeds = uniqueNonEmpty(seeds)
	if len(seeds) == 0 {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	values := make([]string, 0, len(seeds))
	args := make([]any, 0, len(seeds)+1)
	for _, seed := range seeds {
		values = append(values, "(?)")
		args = append(args, seed)
	}
	args = append(args, limit)
	query := fmt.Sprintf(`WITH seed(slug) AS (VALUES %s),
		edges AS (
			SELECT l.to_slug AS slug,
				CASE WHEN l.link_source = 'explicit' THEN 0.80 ELSE 1.20 END AS score
			FROM links l
			JOIN seed ON seed.slug = l.from_slug
			UNION ALL
			SELECT l.from_slug AS slug,
				CASE WHEN l.link_source = 'explicit' THEN 0.90 ELSE 1.30 END AS score
			FROM links l
			JOIN seed ON seed.slug = l.to_slug
		),
		ranked AS (
			SELECT slug, MIN(score) AS score
			FROM edges
			WHERE slug NOT IN (SELECT slug FROM seed) AND slug NOT LIKE 'dreams/%%'
			GROUP BY slug
			ORDER BY score
			LIMIT ?
		)
		SELECT r.slug, COALESCE(c.chunk_index, 0), p.title,
			COALESCE(substr(c.body, 1, 600), p.title) AS snippet,
			r.score
		FROM ranked r
		JOIN pages p ON p.slug = r.slug
		LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
		ORDER BY r.score, p.title`, strings.Join(values, ","))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanSearchRows(rows)
}

func (s *Store) searchFTS(ctx context.Context, match string, limit int) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT f.slug, f.chunk_index, p.title,
		substr(f.body, 1, 600) AS snippet,
		bm25(chunks_fts) AS rank
		FROM chunks_fts f
		JOIN pages p ON p.slug = f.slug
		WHERE chunks_fts MATCH ? AND f.slug NOT LIKE 'dreams/%'
		ORDER BY rank
		LIMIT ?`, match, chunkPoolLimit(limit))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	results, err := scanSearchRows(rows)
	if err != nil {
		return nil, err
	}
	return bestPerPage(results, limit), nil
}

func (s *Store) searchTitleAliasTerm(ctx context.Context, term string, limit int) ([]SearchResult, error) {
	like := "%" + term + "%"
	rows, err := s.searchQ.SearchTitleAliasTerm(ctx, searchdb.SearchTitleAliasTermParams{
		Title:             term,
		Title_2:           like,
		Title_3:           term,
		Title_4:           like,
		NormalizedAlias:   term,
		NormalizedAlias_2: like,
		NormalizedAlias_3: term,
		NormalizedAlias_4: like,
		Limit:             int64(limit),
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, SearchResult{
			Slug:       row.Slug,
			Title:      row.Title,
			ChunkIndex: int(row.ChunkIndex),
			Snippet:    row.Snippet,
			Score:      row.Score,
		})
	}
	return results, nil
}

func (s *Store) searchLike(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	like := "%" + query + "%"
	rows, err := s.searchQ.SearchLike(ctx, searchdb.SearchLikeParams{
		Title:   like,
		Title_2: like,
		Body:    like,
		Limit:   int64(chunkPoolLimit(limit)),
	})
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, SearchResult{
			Slug:       row.Slug,
			Title:      row.Title,
			ChunkIndex: int(row.ChunkIndex),
			Snippet:    row.Snippet,
			Score:      row.Score,
		})
	}
	return bestPerPage(results, limit), nil
}

func scanSearchRows(rows *sql.Rows) ([]SearchResult, error) {
	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.Slug, &result.ChunkIndex, &result.Title, &result.Snippet, &result.Score); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func sortSearchResults(results []SearchResult) {
	sort.Slice(results, func(a, b int) bool {
		if results[a].Score == results[b].Score {
			return results[a].Slug < results[b].Slug
		}
		return results[a].Score < results[b].Score
	})
}

func bestPerPage(rows []SearchResult, limit int) []SearchResult {
	limit = normalizeLimit(limit)
	bySlug := map[string]SearchResult{}
	for _, row := range rows {
		current, ok := bySlug[row.Slug]
		if !ok || row.Score < current.Score || (row.Score == current.Score && row.ChunkIndex < current.ChunkIndex) {
			bySlug[row.Slug] = row
		}
	}
	results := make([]SearchResult, 0, len(bySlug))
	for _, row := range bySlug {
		results = append(results, row)
	}
	sortSearchResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}
