package sqlite

import (
	"context"
	"strings"
	"time"
)

type PageMeta struct {
	Type       string
	ModifiedAt time.Time
}

type LinkRef struct {
	Slug   string `json:"slug"`
	Type   string `json:"type"`
	Source string `json:"source"`
}

// PageMetas stays handwritten: the IN clause is built from a dynamic seed set.
func (s *Store) PageMetas(ctx context.Context, slugs []string) (map[string]PageMeta, error) {
	slugs = uniqueNonEmpty(slugs)
	metas := make(map[string]PageMeta, len(slugs))
	if len(slugs) == 0 {
		return metas, nil
	}
	args := make([]any, 0, len(slugs))
	for _, slug := range slugs {
		args = append(args, slug)
	}
	query := `SELECT slug, type, modified_at_ms FROM pages WHERE slug IN (?` +
		strings.Repeat(",?", len(slugs)-1) + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var slug, pageType string
		var modifiedMs int64
		if err := rows.Scan(&slug, &pageType, &modifiedMs); err != nil {
			return nil, err
		}
		metas[slug] = PageMeta{Type: pageType, ModifiedAt: time.UnixMilli(modifiedMs).UTC()}
	}
	return metas, rows.Err()
}

func (s *Store) PageLinks(ctx context.Context, slug string) (outgoing, incoming []LinkRef, err error) {
	out, err := s.graphQ.OutgoingLinks(ctx, slug)
	if err != nil {
		return nil, nil, err
	}
	for _, row := range out {
		outgoing = append(outgoing, LinkRef{Slug: row.ToSlug, Type: row.LinkType, Source: row.LinkSource})
	}
	in, err := s.graphQ.IncomingLinks(ctx, slug)
	if err != nil {
		return nil, nil, err
	}
	for _, row := range in {
		incoming = append(incoming, LinkRef{Slug: row.FromSlug, Type: row.LinkType, Source: row.LinkSource})
	}
	return outgoing, incoming, nil
}
