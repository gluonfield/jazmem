package indexer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gluonfield/jazmem/internal/memfs"
	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
)

func TestIncrementalIndexTracksContentCatalogAndDeletion(t *testing.T) {
	fs := memfs.New(t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	store, err := sqlitestore.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	observer, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	for _, statement := range []string{
		"CREATE TABLE index_writes (slug TEXT)",
		"CREATE TRIGGER record_index_write AFTER INSERT ON pages BEGIN INSERT INTO index_writes VALUES (new.slug); END",
	} {
		if _, err := observer.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	index := &Indexer{FS: fs, Store: store}
	write := func(slug, body string) {
		t.Helper()
		if err := fs.WritePage(slug, body); err != nil {
			t.Fatal(err)
		}
	}
	run := func(writes, pages, unresolved int) Report {
		t.Helper()
		if _, err := observer.Exec("DELETE FROM index_writes"); err != nil {
			t.Fatal(err)
		}
		report, err := index.Reindex(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var got int
		if err := observer.QueryRow("SELECT COUNT(*) FROM index_writes").Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != writes || report.PageCount != pages || report.UnresolvedLinks != unresolved {
			t.Fatalf("writes=%d want=%d report=%+v want pages=%d unresolved=%d", got, writes, report, pages, unresolved)
		}
		return report
	}
	write("people/alice", "---\ntitle: Alice Jones\naliases: [Alicia]\n---\n# Alice\n")
	write("sources/chat/current", "# Chat Current\nAlicia wrote roadmap. [[Ally]] [[people/future]]\n")
	write("dreams/review/generated", "# Derived report\nAlicia friend.\n")
	if report := run(2, 2, 2); report.MentionLinks != 1 {
		t.Fatalf("mentions=%d", report.MentionLinks)
	}
	run(0, 2, 2)
	index = &Indexer{FS: fs, Store: store}
	run(0, 2, 2)
	write("sources/chat/current", "# Chat Current\nAlicia changed the roadmap. [[Ally]] [[people/future]]\n")
	run(1, 2, 2)
	write("people/alice", "---\ntitle: Alice Jones\naliases: [Alicia, Ally]\n---\n# Alice\n")
	run(2, 2, 1)
	write("people/future", "# Future Person\n")
	run(3, 3, 0)
	write("people/other", "---\ntitle: Someone Else\naliases: [Ally]\n---\n# Other\n")
	run(4, 4, 1)
	if err := os.Remove(filepath.Join(fs.Root, "people/other.md")); err != nil {
		t.Fatal(err)
	}
	run(3, 3, 0)
	write("sources/chat/current", "---\nproject: people/alice\n---\n# Chat Current\nAlicia changed the roadmap. [[Ally]] [[people/future]]\n")
	run(1, 3, 0)
	if err := os.Remove(filepath.Join(fs.Root, "people/alice.md")); err != nil {
		t.Fatal(err)
	}
	run(2, 2, 2)
	var stale int
	if err := observer.QueryRow("SELECT COUNT(*) FROM links WHERE to_slug = 'people/alice'").Scan(&stale); err != nil || stale != 0 {
		t.Fatalf("stale links=%d err=%v", stale, err)
	}
	if _, err := fs.ReadPage("dreams/review/generated"); err != nil {
		t.Fatalf("report must remain directly readable: %v", err)
	}
}

func TestCanceledIndexDoesNotReadFilesystem(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	index := Indexer{FS: memfs.New("missing")}
	if _, err := index.Reindex(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation before reading files, got %v", err)
	}
}
