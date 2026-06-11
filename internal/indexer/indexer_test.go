package indexer

import (
	"testing"

	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
)

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

func TestExtractExplicitLinksMarksRelationshipSectionOnly(t *testing.T) {
	body := StripCode("# Alice\n\n[[people/bob]] appears in prose.\n\n## Relationships\n\n- [[people/cara]] - friend. [Source: [[sources/chat/alice]]]\n\n### Advisors\n\n- [[people/drew]] - advises.\n\n## Notes\n\n- [[people/erin]] appears in notes.\n")
	links := ExtractExplicitLinks(body)
	if len(links) != 5 {
		t.Fatalf("expected five links, got %#v", links)
	}
	byTarget := map[string]bool{}
	for _, link := range links {
		byTarget[link.Target] = link.InRelationships
	}
	if byTarget["people/bob"] {
		t.Fatalf("prose link marked as relationship: %#v", links)
	}
	if !byTarget["people/cara"] || !byTarget["people/drew"] {
		t.Fatalf("relationship section links not marked: %#v", links)
	}
	if byTarget["sources/chat/alice"] {
		t.Fatalf("source citation link marked as relationship: %#v", links)
	}
	if byTarget["people/erin"] {
		t.Fatalf("post-relationship notes link marked as relationship: %#v", links)
	}
}

func TestNormalizeAlias(t *testing.T) {
	if got := NormalizeAlias("  Alice   Smith "); got != "alice smith" {
		t.Fatalf("NormalizeAlias = %q", got)
	}
}

func TestInferTypedLinksDropsUnorientableEdges(t *testing.T) {
	// "founder" prose between two people must not emit a founder_of edge.
	links := inferTypedLinks("people/augustinas", "people/irwin", ExplicitLink{
		Context: "- [[people/irwin]] - student founder, and Oxford AI/robotics routing.",
	})
	if len(links) != 0 {
		t.Fatalf("person-to-person founder context must not produce typed links, got %#v", links)
	}
}

func TestInferRelationDirectorMapsToWorksAt(t *testing.T) {
	links := inferTypedLinks("people/irwin", "companies/oxford-edge", ExplicitLink{
		Context: "- [[companies/oxford-edge]] - Director of Oxford Edge.",
	})
	if len(links) != 1 || links[0].LinkType != "works_at" || links[0].FromSlug != "people/irwin" || links[0].ToSlug != "companies/oxford-edge" {
		t.Fatalf("director context should orient person->org works_at, got %#v", links)
	}
}

func TestDedupeLinksCollapsesDuplicateEdges(t *testing.T) {
	links := dedupeLinks([]sqlitestore.LinkRecord{
		{FromSlug: "people/a", ToSlug: "people/b", LinkType: "friend", LinkSource: "relationship", Context: "ctx one"},
		{FromSlug: "people/a", ToSlug: "people/b", LinkType: "friend", LinkSource: "relationship", Context: "ctx two"},
		{FromSlug: "people/b", ToSlug: "people/a", LinkType: "friend", LinkSource: "relationship", Context: "ctx one"},
		{FromSlug: "people/a", ToSlug: "people/b", LinkType: "mention", LinkSource: "mention", Context: "ctx"},
	})
	if len(links) != 3 {
		t.Fatalf("expected duplicate (from,to,type,source) rows collapsed to 3, got %#v", links)
	}
}
