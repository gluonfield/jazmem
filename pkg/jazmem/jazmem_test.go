package jazmem

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRawMarkdownReindexSearchAndDream(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	if err := mem.fs.WritePage("inbox/alice-riley-note", "---\ntitle: Alice Riley note\ntype: inbox\n---\n\n# Alice Riley note\n\nAlice and Riley are friends. They are working on jazmem search.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	results, err := mem.Search(context.Background(), "jazmem search", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Slug != "inbox/alice-riley-note" {
		t.Fatalf("unexpected search results %#v", results)
	}

	report, err := mem.Dream(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RunSlug != "dreams/runs/2026-06-08" || len(report.InputSlugs) == 0 {
		t.Fatalf("unexpected dream report %#v", report)
	}
	if _, err := mem.GetPage(context.Background(), report.RunSlug); err != nil {
		t.Fatalf("dream run page missing: %v", err)
	}
}

func TestReindexFindsExplicitAndMentionLinks(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	write := func(slug, content string) {
		t.Helper()
		if err := mem.fs.WritePage(slug, content); err != nil {
			t.Fatal(err)
		}
	}
	write("people/alice", "---\ntitle: Alice Smith\naliases: [Alice]\n---\n\n# Alice Smith\n")
	write("people/riley", "---\ntitle: Riley Jones\naliases: [Riley]\n---\n\n# Riley Jones\n")
	write("notes/friendship", "# Friendship\n\n[[people/alice|Alice]] and Riley are friends.\n")

	report, err := mem.Reindex(context.Background(), ReindexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.ExplicitLinks != 1 {
		t.Fatalf("explicit links = %d, want 1", report.ExplicitLinks)
	}
	if report.MentionLinks < 1 {
		t.Fatalf("expected at least one mention link, report %#v", report)
	}
	doctor, err := mem.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doctor.LinkCount < 2 {
		t.Fatalf("expected indexed links, doctor %#v", doctor)
	}
}

func TestSearchFallsBackToBroadTokenMatch(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	if err := mem.fs.WritePage("projects/ink", "---\ntitle: Ink\n---\n\n# Ink\n\nInk supports enterprise Claude deployment.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("people/majid-yazdani", "---\ntitle: Majid Yazdani\naliases: [Majid]\n---\n\n# Majid Yazdani\n\nMajid is connected to Leeroo strategy.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	results, err := mem.Search(context.Background(), "Ink enterprise Claude deployment Majid Leeroo", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected broad token fallback results, got %#v", results)
	}
	slugs := map[string]bool{}
	for _, result := range results {
		slugs[result.Slug] = true
	}
	if !slugs["projects/ink"] || !slugs["people/majid-yazdani"] {
		t.Fatalf("missing expected slugs from broad results: %#v", results)
	}
}

func TestLinkHygieneWritesRelationshipReview(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	if err := mem.fs.WritePage("people/a", "---\ntitle: A\n---\n\n# A\n\nA and R are friends.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("people/r", "---\ntitle: R\n---\n\n# R\n"); err != nil {
		t.Fatal(err)
	}

	report, err := mem.LinkHygiene(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.RelationshipsAdded != 0 {
		t.Fatalf("relationships added = %d, want 0", report.RelationshipsAdded)
	}
	if report.ProposalCount != 1 || report.ReviewSlug != "dreams/review/link-hygiene-2026-06-08" {
		t.Fatalf("unexpected hygiene report %#v", report)
	}
	a, err := mem.GetPage(context.Background(), "people/a")
	if err != nil {
		t.Fatal(err)
	}
	r, err := mem.GetPage(context.Background(), "people/r")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a.Raw, "[[people/r]] - friend") {
		t.Fatalf("A page was mutated:\n%s", a.Raw)
	}
	if strings.Contains(r.Raw, "[[people/a]] - friend") {
		t.Fatalf("R page was mutated:\n%s", r.Raw)
	}
	review, err := mem.GetPage(context.Background(), report.ReviewSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(review.Raw, "[[people/r]] - friend") || !strings.Contains(review.Raw, "[[people/a]] - friend") {
		t.Fatalf("review missing proposal bullets:\n%s", review.Raw)
	}
}

func TestGetPageNotFoundSuggestsSimilarSlugs(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	if err := mem.fs.WritePage("people/alice-bentick", "---\ntitle: Alice Bentick\naliases: [Alice]\n---\n\n# Alice Bentick\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("people/bob", "---\ntitle: Bob\n---\n\n# Bob\n"); err != nil {
		t.Fatal(err)
	}

	_, err = mem.GetPage(context.Background(), "people/alice")
	if err == nil {
		t.Fatal("expected not found error")
	}
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected NotFoundError, got %T: %v", err, err)
	}
	if len(notFound.Suggestions) == 0 || notFound.Suggestions[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected suggestions %#v", notFound.Suggestions)
	}
	if !strings.Contains(err.Error(), "people/alice") {
		t.Fatalf("error did not include requested slug: %v", err)
	}
}
