package indexer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/gluonfield/jazmem/internal/memfs"
)

func (i *Indexer) Reindex(ctx context.Context) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	pages, err := i.FS.ListPages(ctx)
	if err != nil {
		return Report{}, err
	}
	previous, oldCatalog, err := i.Store.IndexSnapshot(ctx)
	if err != nil {
		return Report{}, err
	}
	catalog := catalogHash(pages)
	var changed []memfs.Page
	var removed []string
	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		metadata, err := json.Marshal(page.Frontmatter)
		if err != nil {
			return Report{}, err
		}
		old, exists := previous[page.Slug]
		if catalog != oldCatalog || !exists || old.BodyHash != page.BodyHash || old.FrontmatterJSON != string(metadata) || old.Path != page.RelPath || old.ModifiedAt.UnixMilli() != page.ModifiedAt.UnixMilli() || old.ExtractorHash != extractorHash {
			changed = append(changed, page)
			if exists {
				removed = append(removed, page.Slug)
			}
		}
		delete(previous, page.Slug)
	}
	for slug := range previous {
		removed = append(removed, slug)
	}
	if len(changed) > 0 || len(removed) > 0 {
		data, err := buildIndex(ctx, changed, pages)
		if err != nil {
			return Report{}, err
		}
		if err := i.Store.UpdateIndex(ctx, data, removed, catalog); err != nil {
			return Report{}, err
		}
	}
	return i.Store.IndexCounts(ctx)
}

func catalogHash(pages []memfs.Page) string {
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for _, page := range pages {
		_ = encoder.Encode([]any{page.Slug, page.Type, aliasesForPage(page)})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
