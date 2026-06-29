package jazmem

import (
	"context"
	"sync"
	"time"

	"github.com/gluonfield/jazmem/internal/dream"
	"github.com/gluonfield/jazmem/internal/hygiene"
	"github.com/gluonfield/jazmem/internal/indexer"
	"github.com/gluonfield/jazmem/internal/llm"
	"github.com/gluonfield/jazmem/internal/memfs"
	"github.com/gluonfield/jazmem/internal/search"
	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
)

type Memory struct {
	root   string
	dbPath string
	now    func() time.Time

	fs               *memfs.FileSystem
	store            *sqlitestore.Store
	indexer          *indexer.Indexer
	search           *search.Service
	dream            *dream.Service
	dreamRun         DreamRunner
	noProviderDreams bool
	hygiene          *hygiene.Service
	llm              *llm.Client

	maintenanceMu sync.Mutex
}

func Open(cfg Config) (*Memory, error) {
	cfg = ResolveConfig(cfg)
	if _, err := ensureLayoutResolved(cfg); err != nil {
		return nil, err
	}
	return open(cfg)
}

func open(cfg Config) (*Memory, error) {
	root := cfg.Root
	dbPath := cfg.DBPath
	fs := memfs.New(root)
	store, err := sqlitestore.Open(dbPath)
	if err != nil {
		return nil, err
	}
	m := &Memory{
		root:             root,
		dbPath:           dbPath,
		now:              cfg.Now,
		fs:               fs,
		store:            store,
		noProviderDreams: cfg.DisableProviderDreams,
	}
	m.dreamRun = cfg.DreamRunner
	m.indexer = &indexer.Indexer{FS: fs, Store: store}
	m.search = &search.Service{Store: store}
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

func (m *Memory) SetDreamRunner(runner DreamRunner) {
	m.dreamRun = runner
}

func (m *Memory) timeNow() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}
