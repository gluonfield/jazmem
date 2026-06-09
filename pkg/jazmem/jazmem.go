package jazmem

import (
	"context"
	"time"

	"github.com/wins/jazmem/internal/dream"
	"github.com/wins/jazmem/internal/hygiene"
	"github.com/wins/jazmem/internal/indexer"
	"github.com/wins/jazmem/internal/ingest"
	"github.com/wins/jazmem/internal/llm"
	"github.com/wins/jazmem/internal/memfs"
	"github.com/wins/jazmem/internal/search"
	sqlitestore "github.com/wins/jazmem/internal/store/sqlite"
)

type Memory struct {
	root   string
	dbPath string
	now    func() time.Time

	fs       *memfs.FileSystem
	store    *sqlitestore.Store
	indexer  *indexer.Indexer
	search   *search.Service
	dream    *dream.Service
	hygiene  *hygiene.Service
	ingester *ingest.Service
	llm      *llm.Client
}

func Open(cfg Config) (*Memory, error) {
	cfg = ResolveConfig(cfg)
	root := cfg.Root
	dbPath := cfg.DBPath
	fs := memfs.New(root)
	if err := fs.EnsureLayout(); err != nil {
		return nil, err
	}
	store, err := sqlitestore.Open(dbPath)
	if err != nil {
		return nil, err
	}
	m := &Memory{
		root:   root,
		dbPath: dbPath,
		now:    cfg.Now,
		fs:     fs,
		store:  store,
	}
	m.indexer = &indexer.Indexer{FS: fs, Store: store}
	m.search = &search.Service{Store: store}
	m.ingester = &ingest.Service{}
	m.llm = llm.New(llm.Config{
		APIKey:          cfg.APIKey,
		Model:           cfg.Model,
		Endpoint:        cfg.ProviderEndpoint,
		ReasoningEffort: cfg.ReasoningEffort,
	})
	m.dream = &dream.Service{
		FS:  fs,
		Now: m.timeNow,
		LLM: m.llm,
		Reindex: func(ctx context.Context) error {
			_, err := m.Reindex(ctx, ReindexOptions{})
			return err
		},
	}
	m.hygiene = &hygiene.Service{
		FS:  fs,
		Now: m.timeNow,
		Reindex: func(ctx context.Context) error {
			_, err := m.Reindex(ctx, ReindexOptions{})
			return err
		},
	}
	return m, nil
}

func (m *Memory) Close() error {
	return m.store.Close()
}

func (m *Memory) Root() string {
	return m.root
}

func (m *Memory) DBPath() string {
	return m.dbPath
}

func (m *Memory) timeNow() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}
