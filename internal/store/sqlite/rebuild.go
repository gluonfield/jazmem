package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gluonfield/jazmem/internal/store/sqlite/generated/indexdb"
)

func (s *Store) Rebuild(ctx context.Context, data IndexData) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollback(tx)
	q := indexdb.New(tx)
	for _, clear := range []func(context.Context) error{
		q.ClearChunksFTS,
		q.ClearChunks,
		q.ClearUnresolvedLinks,
		q.ClearLinks,
		q.ClearAliases,
		q.ClearPages,
	} {
		if err := clear(ctx); err != nil {
			return err
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
