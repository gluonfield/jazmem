package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrateFreshDB(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	if version := gooseVersion(t, store.db); version != 1 {
		t.Fatalf("goose version = %d, want 1", version)
	}
	for _, table := range []string{"pages", "aliases", "links", "unresolved_links", "chunks", "chunks_fts", "scheduler_state", "index_state"} {
		exists, err := store.tableExists(t.Context(), table)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing migrated table %s", table)
		}
	}
}

func TestMigrateAdoptsCompleteLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db := openRawSQLite(t, path)
	createLegacySchema(t, db)
	closeDB(t, db)

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if version := gooseVersion(t, store.db); version != 1 {
		t.Fatalf("goose version = %d, want 1", version)
	}
}

func TestMigrateRejectsPartialLegacySchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.sqlite")
	db := openRawSQLite(t, path)
	execSQL(t, db, `CREATE TABLE pages(slug TEXT PRIMARY KEY)`)
	closeDB(t, db)

	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "partially initialized") {
		t.Fatalf("Open error = %v, want partial legacy schema error", err)
	}
}

func TestRebuildSearchEntityAndState(t *testing.T) {
	store := openTestStore(t)
	defer func() { _ = store.Close() }()

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	err := store.Rebuild(t.Context(), IndexData{
		Pages: []PageRecord{
			{Slug: "people/augustinas", Path: "/memory/people/augustinas.md", Type: "people", Title: "Augustinas Malinauskas", AliasesJSON: `["Augustinas"]`, Frontmatter: map[string]any{"title": "Augustinas Malinauskas"}, BodyHash: "a", ModifiedAt: now, IndexedAt: now, ExtractorHash: "v1"},
			{Slug: "concepts/go-stack", Path: "/memory/concepts/go-stack.md", Type: "concepts", Title: "Go Stack", AliasesJSON: `[]`, Frontmatter: map[string]any{"title": "Go Stack"}, BodyHash: "b", ModifiedAt: now, IndexedAt: now, ExtractorHash: "v1"},
		},
		Aliases: []AliasRecord{
			{Slug: "people/augustinas", Alias: "Augustinas", NormalizedAlias: "augustinas"},
		},
		Links: []LinkRecord{
			{FromSlug: "people/augustinas", ToSlug: "concepts/go-stack", LinkType: "uses", LinkSource: "relationship", Display: "Go stack", Context: "Augustinas uses Go for backend systems."},
		},
		Chunks: []ChunkRecord{
			{Slug: "people/augustinas", Index: 0, Title: "Augustinas Malinauskas", Body: "Augustinas builds backend systems in Go.", BodyHash: "c", ModifiedAt: now},
			{Slug: "concepts/go-stack", Index: 0, Title: "Go Stack", Body: "Go, Postgres, sqlc, and Temporal are the backend defaults.", BodyHash: "d", ModifiedAt: now},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	titleHits, err := store.SearchTitleAlias(t.Context(), "tell me about Augustinas", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(titleHits) == 0 || titleHits[0].Slug != "people/augustinas" {
		t.Fatalf("title alias hits = %#v, want people/augustinas first", titleHits)
	}

	ftsHits, err := store.Search(t.Context(), "Postgres sqlc", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ftsHits) == 0 || ftsHits[0].Slug != "concepts/go-stack" {
		t.Fatalf("fts hits = %#v, want concepts/go-stack first", ftsHits)
	}

	resolved, err := store.ResolveEntity(t.Context(), "Augustinas")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "people/augustinas" {
		t.Fatalf("resolved = %q, want people/augustinas", resolved)
	}

	related, err := store.RelationalFanout(t.Context(), "people/augustinas", []string{"uses"}, "out", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(related) == 0 || related[0].Slug != "concepts/go-stack" {
		t.Fatalf("related = %#v, want concepts/go-stack", related)
	}

	doctor, err := store.Doctor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if doctor.PageCount != 2 || doctor.ChunkCount != 2 || doctor.TypedLinkCount != 1 {
		t.Fatalf("doctor = %#v", doctor)
	}

	if err := store.RecordTask(t.Context(), "index", "ok", now, ""); err != nil {
		t.Fatal(err)
	}
	lastRun, status, err := store.TaskState(t.Context(), "index")
	if err != nil {
		t.Fatal(err)
	}
	if !lastRun.Equal(now) || status != "ok" {
		t.Fatalf("task state = %s %q, want %s ok", lastRun, status, now)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func openRawSQLite(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func closeDB(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func execSQL(t *testing.T, db *sql.DB, stmt string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), stmt); err != nil {
		t.Fatal(err)
	}
}

func gooseVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var version int
	if err := db.QueryRowContext(t.Context(), `SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func createLegacySchema(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		`CREATE TABLE pages (
			slug TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			aliases_json TEXT NOT NULL,
			body_hash TEXT NOT NULL,
			frontmatter_json TEXT NOT NULL,
			modified_at_ms INTEGER NOT NULL,
			indexed_at_ms INTEGER NOT NULL,
			extractor_hash TEXT NOT NULL
		)`,
		`CREATE TABLE aliases (
			slug TEXT NOT NULL,
			alias TEXT NOT NULL,
			normalized_alias TEXT NOT NULL,
			PRIMARY KEY (slug, normalized_alias)
		)`,
		`CREATE TABLE links (
			from_slug TEXT NOT NULL,
			to_slug TEXT NOT NULL,
			link_type TEXT NOT NULL,
			link_source TEXT NOT NULL,
			display TEXT NOT NULL,
			context TEXT NOT NULL,
			PRIMARY KEY (from_slug, to_slug, link_type, link_source, display, context)
		)`,
		`CREATE TABLE unresolved_links (
			from_slug TEXT NOT NULL,
			target TEXT NOT NULL,
			display TEXT NOT NULL,
			reason TEXT NOT NULL,
			context TEXT NOT NULL,
			PRIMARY KEY (from_slug, target, display, reason, context)
		)`,
		`CREATE TABLE chunks (
			slug TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			body TEXT NOT NULL,
			body_hash TEXT NOT NULL,
			embedding BLOB,
			modified_at_ms INTEGER NOT NULL,
			PRIMARY KEY (slug, chunk_index)
		)`,
		`CREATE VIRTUAL TABLE chunks_fts USING fts5(
			slug UNINDEXED,
			chunk_index UNINDEXED,
			title,
			body
		)`,
		`CREATE TABLE scheduler_state (
			task TEXT PRIMARY KEY,
			last_run_at_ms INTEGER NOT NULL,
			last_status TEXT NOT NULL,
			last_error TEXT NOT NULL
		)`,
		`CREATE TABLE index_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`,
	} {
		execSQL(t, db, stmt)
	}
}
