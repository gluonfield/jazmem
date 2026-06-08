package indexer

import "testing"

func TestExtractExplicitLinksIgnoresCode(t *testing.T) {
	body := StripCode("Use [[people/alice|Alice]].\n\n```md\n[[people/bob]]\n```\n`[[people/cara]]`")
	links := ExtractExplicitLinks(body)
	if len(links) != 1 {
		t.Fatalf("expected one link, got %#v", links)
	}
	if links[0].Target != "people/alice" || links[0].Display != "Alice" {
		t.Fatalf("unexpected link %#v", links[0])
	}
}

func TestNormalizeAlias(t *testing.T) {
	if got := NormalizeAlias("  Alice   Smith "); got != "alice smith" {
		t.Fatalf("NormalizeAlias = %q", got)
	}
}
