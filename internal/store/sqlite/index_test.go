package sqlite

import "testing"

func TestIndexUpdateRollsBackDeletionsOnFailure(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()
	page := PageRecord{Slug: "people/alice", Title: "Alice", AliasesJSON: "[]"}
	data := IndexData{
		Pages:   []PageRecord{page},
		Aliases: []AliasRecord{{Slug: page.Slug, Alias: "Alice", NormalizedAlias: "alice"}},
		Chunks:  []ChunkRecord{{Slug: page.Slug, Body: "original content"}},
	}
	if err := store.UpdateIndex(t.Context(), data, nil, "original"); err != nil {
		t.Fatal(err)
	}
	broken := IndexData{Pages: []PageRecord{page, page}}
	if err := store.UpdateIndex(t.Context(), broken, []string{page.Slug}, "broken"); err == nil {
		t.Fatal("duplicate page should abort the transaction")
	}
	_, catalog, err := store.IndexSnapshot(t.Context())
	if err != nil || catalog != "original" {
		t.Fatalf("catalog=%q err=%v", catalog, err)
	}
	hits, err := store.Search(t.Context(), "original", 5)
	if err != nil || len(hits) != 1 || hits[0].Slug != page.Slug {
		t.Fatalf("original search entry lost: hits=%v err=%v", hits, err)
	}
	entity, err := store.ResolveEntity(t.Context(), "Alice")
	if err != nil || entity != page.Slug {
		t.Fatalf("original alias lost: entity=%q err=%v", entity, err)
	}
}
