package hygiene

import (
	"testing"
	"time"

	"github.com/gluonfield/jazmem/internal/memfs"
)

func TestReportsCannotBecomeRelationshipEvidence(t *testing.T) {
	fs := memfs.New(t.TempDir())
	pages := map[string]string{
		"people/alice":      "# Alice Smith\n",
		"people/bob":        "# Bob Jones\n",
		"people/cara":       "# Cara West\n",
		"people/drew":       "# Drew Hill\n",
		"sources/first":     "# First\nAlice Smith is a friend of Bob Jones.\n",
		"sources/second":    "# Second\nCara West is a friend of Drew Hill.\n",
		"sources/unrelated": "# Unrelated\nAlice Smith mentioned a friend.\nDrew Hill wrote something else.\n",
	}
	for slug, body := range pages {
		if err := fs.WritePage(slug, body); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	service := Service{
		FS: fs,
		Now: func() time.Time {
			return now
		},
	}
	for range 2 {
		report, err := service.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if report.ProposalCount != 2 {
			t.Fatalf("proposals=%d want=2", report.ProposalCount)
		}
		for _, proposal := range report.Proposals {
			if proposal.SourceSlug != "sources/first" && proposal.SourceSlug != "sources/second" {
				t.Fatalf("invalid evidence source %q", proposal.SourceSlug)
			}
		}
		now = now.Add(24 * time.Hour)
	}
}
