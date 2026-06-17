package agenticprompt

import (
	"strings"
	"testing"
)

func TestRenderUserKeepsEvidenceBoundary(t *testing.T) {
	got, err := RenderUser(UserData{
		Query: "who is Alice?",
		Evidence: []Evidence{{
			ID:      1,
			Slug:    "people/alice",
			Title:   "Alice",
			Chunk:   2,
			Snippet: "Alice is working on Acme follow-up.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "Evidence:\n\n[1] slug=people/alice title=Alice chunk=2"; !strings.Contains(got, want) {
		t.Fatalf("rendered user prompt missing %q:\n%s", want, got)
	}
	if strings.Contains(got, "Evidence:[") {
		t.Fatalf("rendered user prompt joined evidence label and first item:\n%s", got)
	}
}
