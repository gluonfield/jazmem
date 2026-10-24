package indexer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

func TestIncrementalRelationshipsFollowTheirEvidence(t *testing.T) {
	for _, test := range []struct {
		name    string
		initial map[string]string
		edits   map[string]string
		want    []string
	}{
		{
			name: "remove symmetric evidence",
			initial: map[string]string{
				"people/alice": "# Alice\n## Relationships\n- [[people/bob]] - friend.\n",
				"people/bob":   "# Bob\n",
			},
			edits: map[string]string{"people/alice": "# Alice\nNo current relationship.\n"},
		},
		{
			name: "edit destination preserves evidence elsewhere",
			initial: map[string]string{
				"people/alice": "# Alice\n## Relationships\n- [[people/bob]] - friend.\n",
				"people/bob":   "# Bob\n",
			},
			edits: map[string]string{"people/bob": "# Bob\nUpdated biography.\n"},
			want:  []string{"people/alice people/bob friend", "people/bob people/alice friend"},
		},
		{
			name: "remove reversed evidence",
			initial: map[string]string{
				"companies/acme": "# Acme\n## Relationships\n- [[people/alice]] - works at.\n",
				"people/alice":   "# Alice\n",
			},
			edits: map[string]string{"companies/acme": "# Acme\nNo current employees.\n"},
		},
		{
			name: "reciprocal evidence stays deduplicated",
			initial: map[string]string{
				"people/alice": "# Alice\n## Relationships\n- [[people/bob]] - friend.\n",
				"people/bob":   "# Bob\n## Relationships\n- [[people/alice]] - friend.\n",
			},
			edits: map[string]string{"people/alice": "# Alice\n## Relationships\n- [[people/bob]] - close friend.\n"},
			want:  []string{"people/alice people/bob friend", "people/bob people/alice friend"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fs := memfs.New(t.TempDir())
			dbPath := filepath.Join(t.TempDir(), "index.sqlite")
			store, err := sqlitestore.Open(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			index := &Indexer{FS: fs, Store: store}
			for _, pages := range []map[string]string{test.initial, test.edits} {
				for slug, body := range pages {
					if err := fs.WritePage(slug, body); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := index.Reindex(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			observer, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close()
			rows, err := observer.Query("SELECT from_slug || ' ' || to_slug || ' ' || link_type FROM links WHERE link_source = 'relationship' ORDER BY from_slug, to_slug, link_type")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var got []string
			for rows.Next() {
				var edge string
				if err := rows.Scan(&edge); err != nil {
					t.Fatal(err)
				}
				got = append(got, edge)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("relationships=%v want=%v", got, test.want)
			}
		})
	}
}
