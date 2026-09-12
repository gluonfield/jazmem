package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gluonfield/jazmem/internal/store/sqlite/generated/indexdb"
)

func (s *Store) UpdateIndex(ctx context.Context, data IndexData, removed []string, catalog string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	q := indexdb.New(tx)
	if len(removed) > 0 {
		slugs, err := json.Marshal(removed)
		if err != nil {
			return err
		}
		for _, remove := range []func(context.Context, any) error{
			q.DeletePageChunksFTS,
			q.DeletePageChunks,
			q.DeletePageUnresolved,
			q.DeletePageLinks,
			q.DeletePageAliases,
			q.DeletePageIndex,
		} {
			if err := remove(ctx, string(slugs)); err != nil {
				return err
			}
		}
	}
	if err := insertPages(ctx, q, data.Pages); err != nil {
		return err
	}
	if err := insertAliases(ctx, q, data.Aliases); err != nil {
		return err
	}
	if err := insertLinks(ctx, q, data.Links); err != nil {
		return err
	}
	if err := insertUnresolved(ctx, q, data.Unresolved); err != nil {
		return err
	}
	if err := insertChunks(ctx, q, data.Chunks); err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := q.RecordIndexState(ctx, indexdb.RecordIndexStateParams{
		Key:         "last_rebuild",
		Value:       now.Format(time.RFC3339),
		UpdatedAtMs: millis(now),
	}); err != nil {
		return err
	}
	if err := q.RecordIndexState(ctx, indexdb.RecordIndexStateParams{
		Key:         "catalog",
		Value:       catalog,
		UpdatedAtMs: millis(now),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func insertPages(ctx context.Context, q indexdb.Querier, pages []PageRecord) error {
	for _, page := range pages {
		fm, err := json.Marshal(page.Frontmatter)
		if err != nil {
			return err
		}
		if err := q.InsertPage(ctx, indexdb.InsertPageParams{
			Slug:            page.Slug,
			Path:            page.Path,
			Type:            page.Type,
			Title:           page.Title,
			AliasesJson:     page.AliasesJSON,
			BodyHash:        page.BodyHash,
			FrontmatterJson: string(fm),
			ModifiedAtMs:    millis(page.ModifiedAt),
			IndexedAtMs:     millis(page.IndexedAt),
			ExtractorHash:   page.ExtractorHash,
		}); err != nil {
			return err
		}
	}
	return nil
}

func insertAliases(ctx context.Context, q indexdb.Querier, aliases []AliasRecord) error {
	for _, alias := range aliases {
		if err := q.InsertAlias(ctx, indexdb.InsertAliasParams{
			Slug:            alias.Slug,
			Alias:           alias.Alias,
			NormalizedAlias: alias.NormalizedAlias,
		}); err != nil {
			return err
		}
	}
	return nil
}

func insertLinks(ctx context.Context, q indexdb.Querier, links []LinkRecord) error {
	for _, link := range links {
		if link.FromSlug == "" || link.ToSlug == "" || link.FromSlug == link.ToSlug {
			continue
		}
		if err := q.InsertLink(ctx, indexdb.InsertLinkParams{
			FromSlug:   link.FromSlug,
			ToSlug:     link.ToSlug,
			LinkType:   link.LinkType,
			LinkSource: link.LinkSource,
			Display:    link.Display,
			Context:    link.Context,
		}); err != nil {
			return err
		}
	}
	return nil
}

func insertUnresolved(ctx context.Context, q indexdb.Querier, unresolved []UnresolvedLinkRecord) error {
	for _, link := range unresolved {
		if err := q.InsertUnresolvedLink(ctx, indexdb.InsertUnresolvedLinkParams{
			FromSlug: link.FromSlug,
			Target:   link.Target,
			Display:  link.Display,
			Reason:   link.Reason,
			Context:  link.Context,
		}); err != nil {
			return err
		}
	}
	return nil
}

func insertChunks(ctx context.Context, q indexdb.Querier, chunks []ChunkRecord) error {
	for _, chunk := range chunks {
		if err := q.InsertChunk(ctx, indexdb.InsertChunkParams{
			Slug:         chunk.Slug,
			ChunkIndex:   int64(chunk.Index),
			Body:         chunk.Body,
			BodyHash:     chunk.BodyHash,
			Embedding:    chunk.Embedding,
			ModifiedAtMs: millis(chunk.ModifiedAt),
		}); err != nil {
			return err
		}
		if err := q.InsertChunkFTS(ctx, indexdb.InsertChunkFTSParams{
			Slug:       chunk.Slug,
			ChunkIndex: strconv.Itoa(chunk.Index),
			Title:      chunk.Title,
			Body:       chunk.Body,
		}); err != nil {
			return err
		}
	}
	return nil
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

type IndexedPage struct {
	Path            string
	BodyHash        string
	FrontmatterJSON string
	ModifiedAt      time.Time
	ExtractorHash   string
}

func (s *Store) IndexSnapshot(ctx context.Context) (map[string]IndexedPage, string, error) {
	q := indexdb.New(s.db)
	rows, err := q.ListIndexedPages(ctx)
	if err != nil {
		return nil, "", err
	}
	pages := make(map[string]IndexedPage, len(rows))
	for _, row := range rows {
		pages[row.Slug] = IndexedPage{
			Path:            row.Path,
			BodyHash:        row.BodyHash,
			FrontmatterJSON: row.FrontmatterJson,
			ModifiedAt:      time.UnixMilli(row.ModifiedAtMs),
			ExtractorHash:   row.ExtractorHash,
		}
	}
	catalog, err := q.IndexCatalog(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return pages, catalog, err
}

type IndexReport struct {
	PageCount       int `json:"page_count"`
	ChunkCount      int `json:"chunk_count"`
	ExplicitLinks   int `json:"explicit_links"`
	TypedLinks      int `json:"typed_links"`
	MentionLinks    int `json:"mention_links"`
	UnresolvedLinks int `json:"unresolved_links"`
}

func (s *Store) IndexCounts(ctx context.Context) (IndexReport, error) {
	counts, err := indexdb.New(s.db).IndexCounts(ctx)
	return IndexReport{
		PageCount:       int(counts.Pages),
		ChunkCount:      int(counts.Chunks),
		ExplicitLinks:   int(counts.ExplicitLinks),
		TypedLinks:      int(counts.TypedLinks),
		MentionLinks:    int(counts.MentionLinks),
		UnresolvedLinks: int(counts.UnresolvedLinks),
	}, err
}
