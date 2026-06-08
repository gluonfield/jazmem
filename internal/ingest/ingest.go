package ingest

import (
	"context"
)

type Source interface {
	Name() string
	Ingest(ctx context.Context) ([]ImportedPage, error)
}

type ImportedPage struct {
	Slug    string
	Content string
}

type Service struct {
	Sources []Source
}

func (s *Service) Run(ctx context.Context) ([]ImportedPage, error) {
	var out []ImportedPage
	for _, source := range s.Sources {
		pages, err := source.Ingest(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, pages...)
	}
	return out, nil
}
