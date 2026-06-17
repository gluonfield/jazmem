package dreamprompt

import (
	"strings"
	"testing"
)

func TestRenderSystemIncludesHorizonPolicies(t *testing.T) {
	got, err := RenderSystem(SystemData{
		LongTermPolicy:  "profile-level memory only",
		ShortTermPolicy: "active working set only",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- LONG_TERM.md: profile-level memory only",
		"- SHORT_TERM.md: active working set only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered system prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderUserKeepsSectionBoundaries(t *testing.T) {
	got, err := RenderUser(UserData{
		Date:      "2026-06-17",
		LongTerm:  "# Long Term Memory",
		ShortTerm: "# Short Term Memory",
		Canonical: []Page{{
			Slug:  "people/alice",
			Title: "Alice",
		}},
		Inputs: []Page{{
			Slug:       "daily/2026-06-17",
			Title:      "Daily 2026-06-17",
			ModifiedAt: "2026-06-17T09:00:00Z",
			Body:       "- Learned something durable.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"Canonical pages you may edit:\n- people/alice - Alice",
		"Input pages:\n\n---\nslug: daily/2026-06-17",
		"body:\n- Learned something durable.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered user prompt missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{
		"edit:-",
		"pages:---",
	} {
		if strings.Contains(got, bad) {
			t.Fatalf("rendered user prompt contains joined text %q:\n%s", bad, got)
		}
	}
}
