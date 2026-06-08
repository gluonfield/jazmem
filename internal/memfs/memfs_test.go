package memfs

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseFrontmatterAndTitle(t *testing.T) {
	raw := `---
title: Alice
type: people
aliases: [Alicia, Al]
---

# Ignored Heading

Body text.
`
	page, err := Parse("people/alice.md", filepath.Join("people", "alice.md"), raw, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if page.Slug != "people/alice" || page.Type != "people" || page.Title != "Alice" {
		t.Fatalf("unexpected page identity: %#v", page)
	}
	if len(page.Aliases) != 2 || page.Aliases[0] != "Alicia" || page.Aliases[1] != "Al" {
		t.Fatalf("unexpected aliases %#v", page.Aliases)
	}
	if page.Body == "" || page.BodyHash == "" {
		t.Fatalf("expected body and body hash")
	}
}

func TestPathForSlugRejectsEscape(t *testing.T) {
	fs := New(t.TempDir())
	if _, err := fs.PathForSlug("../outside"); err == nil {
		t.Fatal("expected escaping slug to be rejected")
	}
}
