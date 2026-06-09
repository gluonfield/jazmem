package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gluonfield/jazmem/internal/store/sqlite/generated/entitydb"
	"github.com/gluonfield/jazmem/internal/store/sqlite/generated/statedb"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	entityQ entitydb.Querier
	stateQ  statedb.Querier
}

type IndexData struct {
	Pages      []PageRecord
	Aliases    []AliasRecord
	Links      []LinkRecord
	Unresolved []UnresolvedLinkRecord
	Chunks     []ChunkRecord
}

type PageRecord struct {
	Slug          string
	Path          string
	Type          string
	Title         string
	AliasesJSON   string
	BodyHash      string
	Frontmatter   map[string]any
	ModifiedAt    time.Time
	IndexedAt     time.Time
	ExtractorHash string
}

type AliasRecord struct {
	Slug            string
	Alias           string
	NormalizedAlias string
}

type LinkRecord struct {
	FromSlug   string
	ToSlug     string
	LinkType   string
	LinkSource string
	Display    string
	Context    string
}

type UnresolvedLinkRecord struct {
	FromSlug string
	Target   string
	Display  string
	Reason   string
	Context  string
}

type ChunkRecord struct {
	Slug       string
	Index      int
	Title      string
	Body       string
	BodyHash   string
	Embedding  []byte
	ModifiedAt time.Time
}

type SearchResult struct {
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	ChunkIndex int     `json:"chunk_index"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}

type DoctorReport struct {
	PageCount       int `json:"page_count"`
	ChunkCount      int `json:"chunk_count"`
	LinkCount       int `json:"link_count"`
	TypedLinkCount  int `json:"typed_link_count"`
	UnresolvedCount int `json:"unresolved_count"`
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("sqlite path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{
		db:      db,
		entityQ: entitydb.New(db),
		stateQ:  statedb.New(db),
	}
	if err := store.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) configure() error {
	for _, stmt := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
