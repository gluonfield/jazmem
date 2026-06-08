package dream

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wins/jazmem/internal/memfs"
)

type Service struct {
	FS      *memfs.FileSystem
	Now     func() time.Time
	Reindex func(context.Context) error
}

type Options struct {
	Date time.Time
}

type Report struct {
	RunSlug     string   `json:"run_slug"`
	InputSlugs  []string `json:"input_slugs"`
	Promoted    int      `json:"promoted"`
	ReviewItems int      `json:"review_items"`
	Skipped     int      `json:"skipped"`
}

func (s *Service) Run(ctx context.Context, opts Options) (Report, error) {
	if s.Reindex != nil {
		if err := s.Reindex(ctx); err != nil {
			return Report{}, err
		}
	}
	date := opts.Date
	if date.IsZero() {
		date = s.now()
	}
	date = date.Local()
	pages, err := s.FS.ListPages()
	if err != nil {
		return Report{}, err
	}
	inputs := dreamInputs(pages)
	runSlug := "dreams/runs/" + date.Format("2006-01-02")
	if err := s.FS.WritePage(runSlug, renderRunPage(date, inputs)); err != nil {
		return Report{}, err
	}
	if s.Reindex != nil {
		if err := s.Reindex(ctx); err != nil {
			return Report{}, err
		}
	}
	slugs := make([]string, 0, len(inputs))
	for _, page := range inputs {
		slugs = append(slugs, page.Slug)
	}
	return Report{RunSlug: runSlug, InputSlugs: slugs}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func dreamInputs(pages []memfs.Page) []memfs.Page {
	var inputs []memfs.Page
	for _, page := range pages {
		if isDreamInput(page.Slug) {
			inputs = append(inputs, page)
		}
	}
	sort.Slice(inputs, func(i, j int) bool {
		if inputs[i].ModifiedAt.Equal(inputs[j].ModifiedAt) {
			return inputs[i].Slug < inputs[j].Slug
		}
		return inputs[i].ModifiedAt.After(inputs[j].ModifiedAt)
	})
	if len(inputs) > 100 {
		inputs = inputs[:100]
	}
	return inputs
}

func isDreamInput(slug string) bool {
	for _, prefix := range []string{
		"daily/",
		"inbox/",
		"sources/email/",
		"sources/chat/",
		"sources/agent/",
	} {
		if strings.HasPrefix(slug, prefix) {
			return true
		}
	}
	return false
}

func renderRunPage(date time.Time, inputs []memfs.Page) string {
	var b strings.Builder
	b.WriteString(memfs.FrontmatterString(map[string]string{
		"title": "Dream " + date.Format("2006-01-02"),
		"type":  "dream_run",
		"date":  date.Format("2006-01-02"),
	}))
	fmt.Fprintf(&b, "# Dream %s\n\n", date.Format("2006-01-02"))
	b.WriteString("## Inputs\n\n")
	if len(inputs) == 0 {
		b.WriteString("- No eligible inputs found.\n")
	} else {
		for _, page := range inputs {
			fmt.Fprintf(&b, "- [[%s]] - %s\n", page.Slug, page.Title)
		}
	}
	b.WriteString("\n## Candidates\n\n")
	b.WriteString("- No automatic candidates promoted in this deterministic v1 run.\n")
	b.WriteString("\n## Decisions\n\n")
	b.WriteString("- Promotion requires either explicit link hygiene confidence or a later LLM-backed extractor.\n")
	b.WriteString("\n## Skipped\n\n")
	b.WriteString("- Ambiguous facts and relationships stay out of canonical pages until review.\n")
	return b.String()
}
