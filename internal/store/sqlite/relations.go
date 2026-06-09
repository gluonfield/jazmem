package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/gluonfield/jazmem/internal/store/sqlite/generated/entitydb"
)

func (s *Store) ResolveEntity(ctx context.Context, text string) (string, error) {
	text = cleanEntityPhrase(text)
	if text == "" {
		return "", nil
	}
	slug := cleanSlug(text)
	if slug != "" {
		found, err := s.entityQ.FindPageSlug(ctx, slug)
		if err == nil {
			return found, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	normalized := normalizeEntity(text)
	matches, err := s.entityQ.ResolveEntitySlugs(ctx, entitydb.ResolveEntitySlugsParams{
		NormalizedAlias: normalized,
		Title:           normalized,
	})
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", nil
	}
	return matches[0], nil
}

func (s *Store) RelationalFanout(ctx context.Context, seed string, linkTypes []string, direction string, limit int) ([]SearchResult, error) {
	seed = strings.TrimSpace(seed)
	linkTypes = uniqueNonEmpty(linkTypes)
	if seed == "" || len(linkTypes) == 0 {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	typeWhere, args := linkTypeWhere(linkTypes)
	var edgeSQL string
	switch direction {
	case "out":
		args = append(args, seed)
		edgeSQL = `SELECT to_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND from_slug = ?`
	case "both":
		args = append(args, seed)
		args = append(args, stringSliceToAny(linkTypes)...)
		args = append(args, seed)
		edgeSQL = `SELECT to_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND from_slug = ?
			UNION ALL
			SELECT from_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND to_slug = ?`
	default:
		args = append(args, seed)
		edgeSQL = `SELECT from_slug AS slug, link_type, context, -20.0 AS score
			FROM links
			WHERE link_source = 'relationship' AND ` + typeWhere + ` AND to_slug = ?`
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `WITH edges AS (`+edgeSQL+`),
		ranked AS (
			SELECT slug, MIN(score) AS score, MIN(link_type) AS link_type, MIN(context) AS context
			FROM edges
			WHERE slug <> ''
			GROUP BY slug
			ORDER BY score, slug
			LIMIT ?
		)
		SELECT r.slug, COALESCE(c.chunk_index, 0), p.title,
			CASE
				WHEN r.context <> '' THEN '[' || r.link_type || '] ' || r.context
				ELSE COALESCE(substr(c.body, 1, 600), p.title)
			END AS snippet,
			r.score
		FROM ranked r
		JOIN pages p ON p.slug = r.slug
		LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
		ORDER BY r.score, p.title`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanSearchRows(rows)
}

func (s *Store) RelationalBetween(ctx context.Context, left, right string, limit int) ([]SearchResult, error) {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" || left == right {
		return nil, nil
	}
	limit = normalizeLimit(limit)
	rows, err := s.db.QueryContext(ctx, `WITH undirected AS (
			SELECT from_slug AS a, to_slug AS b, link_type, context
			FROM links
			WHERE link_source = 'relationship'
			UNION ALL
			SELECT to_slug AS a, from_slug AS b, link_type, context
			FROM links
			WHERE link_source = 'relationship'
		),
		candidates AS (
			SELECT ? AS slug, -20.0 AS score, link_type, context
			FROM undirected
			WHERE a = ? AND b = ?
			UNION ALL
			SELECT ? AS slug, -20.0 AS score, link_type, context
			FROM undirected
			WHERE a = ? AND b = ?
			UNION ALL
			SELECT u1.b AS slug, -19.0 AS score, u1.link_type || '+' || u2.link_type AS link_type,
				COALESCE(NULLIF(u1.context, ''), '') || CASE WHEN u1.context <> '' AND u2.context <> '' THEN ' / ' ELSE '' END || COALESCE(NULLIF(u2.context, ''), '') AS context
			FROM undirected u1
			JOIN undirected u2 ON u2.b = u1.b
			WHERE u1.a = ? AND u2.a = ? AND u1.b NOT IN (?, ?)
		),
		ranked AS (
			SELECT slug, MIN(score) AS score, MIN(link_type) AS link_type, MIN(context) AS context
			FROM candidates
			WHERE slug <> ''
			GROUP BY slug
			ORDER BY score, slug
			LIMIT ?
		)
		SELECT r.slug, COALESCE(c.chunk_index, 0), p.title,
			CASE
				WHEN r.context <> '' THEN '[' || r.link_type || '] ' || r.context
				ELSE COALESCE(substr(c.body, 1, 600), p.title)
			END AS snippet,
			r.score
		FROM ranked r
		JOIN pages p ON p.slug = r.slug
		LEFT JOIN chunks c ON c.slug = r.slug AND c.chunk_index = 0
		ORDER BY r.score, p.title`,
		left, left, right,
		right, right, left,
		left, right, left, right,
		limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanSearchRows(rows)
}

func linkTypeWhere(linkTypes []string) (string, []any) {
	if len(linkTypes) == 1 {
		return "link_type = ?", []any{linkTypes[0]}
	}
	parts := make([]string, 0, len(linkTypes))
	args := make([]any, 0, len(linkTypes))
	for _, linkType := range linkTypes {
		parts = append(parts, "?")
		args = append(args, linkType)
	}
	return "link_type IN (" + strings.Join(parts, ",") + ")", args
}

func stringSliceToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}
