package search

import (
	"context"

	sqlitestore "github.com/wins/jazmem/internal/store/sqlite"
)

type Service struct {
	Store *sqlitestore.Store
}

type Options struct {
	Limit int
}

func (s *Service) Search(ctx context.Context, query string, opts Options) ([]sqlitestore.SearchResult, error) {
	return s.Store.Search(ctx, query, opts.Limit)
}
