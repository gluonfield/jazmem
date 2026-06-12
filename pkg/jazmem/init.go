package jazmem

import (
	"context"
)

type InitReport struct {
	Root                  string   `json:"root"`
	DBPath                string   `json:"db_path"`
	Directories           []string `json:"directories"`
	CreatedDirs           []string `json:"created_dirs"`
	ExistingDirs          []string `json:"existing_dirs"`
	IndexedPages          int      `json:"indexed_pages"`
	IndexedChunks         int      `json:"indexed_chunks"`
	ExplicitLinks         int      `json:"explicit_links"`
	MentionLinks          int      `json:"mention_links"`
	UnresolvedLinks       int      `json:"unresolved_links"`
	MarkdownSourceOfTruth bool     `json:"markdown_source_of_truth"`
}

func Init(ctx context.Context, cfg Config) (InitReport, error) {
	cfg = ResolveConfig(cfg)
	layout, err := ensureLayoutResolved(cfg)
	if err != nil {
		return InitReport{}, err
	}
	memory, err := open(cfg)
	if err != nil {
		return InitReport{}, err
	}
	defer func() { _ = memory.Close() }()
	indexReport, err := memory.Reindex(ctx, ReindexOptions{})
	if err != nil {
		return InitReport{}, err
	}
	return InitReport{
		Root:                  memory.Root(),
		DBPath:                memory.DBPath(),
		Directories:           layout.Directories,
		CreatedDirs:           layout.CreatedDirs,
		ExistingDirs:          layout.ExistingDirs,
		IndexedPages:          indexReport.PageCount,
		IndexedChunks:         indexReport.ChunkCount,
		ExplicitLinks:         indexReport.ExplicitLinks,
		MentionLinks:          indexReport.MentionLinks,
		UnresolvedLinks:       indexReport.UnresolvedLinks,
		MarkdownSourceOfTruth: true,
	}, nil
}
