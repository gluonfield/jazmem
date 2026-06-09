package dream

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wins/jazmem/internal/llm"
	"github.com/wins/jazmem/internal/memfs"
)

type Service struct {
	FS      *memfs.FileSystem
	Now     func() time.Time
	LLM     *llm.Client
	Reindex func(context.Context) error
}

type Options struct {
	Date time.Time
}

type Report struct {
	RunSlug     string   `json:"run_slug"`
	ReviewSlug  string   `json:"review_slug,omitempty"`
	InputSlugs  []string `json:"input_slugs"`
	Promoted    int      `json:"promoted"`
	ReviewItems int      `json:"review_items"`
	Skipped     int      `json:"skipped"`
	ModelUsed   string   `json:"model_used,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

type llmDream struct {
	Summary    string           `json:"summary"`
	Promotions []dreamPromotion `json:"promotions"`
	Review     []dreamReview    `json:"review"`
	Skipped    []string         `json:"skipped"`
}

type dreamPromotion struct {
	TargetSlug  string   `json:"target_slug"`
	Section     string   `json:"section"`
	Bullet      string   `json:"bullet"`
	Confidence  string   `json:"confidence"`
	SourceSlugs []string `json:"source_slugs"`
}

type dreamReview struct {
	Candidate   string   `json:"candidate"`
	Reason      string   `json:"reason"`
	SourceSlugs []string `json:"source_slugs"`
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
	inputSlugs := pageSlugs(inputs)
	pageBySlug := pagesBySlug(pages)

	llmResp, err := s.LLM.CompleteJSON(ctx, llm.Request{
		MaxTokens: 3200,
		Messages: []llm.Message{
			{Role: "system", Content: dreamSystemPrompt()},
			{Role: "user", Content: dreamUserPrompt(date, inputs, canonicalPages(pages))},
		},
	})
	if err != nil {
		return Report{}, err
	}
	parsed, warnings := parseDream(llmResp.Content)
	inputSet := slugSet(inputSlugs)
	var promoted []dreamPromotion
	var review []dreamReview
	review = append(review, parsed.Review...)
	for _, promotion := range parsed.Promotions {
		ok, reason := s.applyPromotion(promotion, pageBySlug, inputSet, date)
		if ok {
			promoted = append(promoted, promotion)
			continue
		}
		review = append(review, dreamReview{
			Candidate:   promotion.Bullet,
			Reason:      reason,
			SourceSlugs: promotion.SourceSlugs,
		})
	}

	runSlug := "dreams/runs/" + date.Format("2006-01-02")
	if err := s.FS.WritePage(runSlug, renderRunPage(date, inputSlugs, parsed, promoted, review, warnings, llmResp.Model)); err != nil {
		return Report{}, err
	}
	reviewSlug := ""
	if len(review) > 0 {
		reviewSlug = "dreams/review/dream-" + date.Format("2006-01-02")
		if err := s.FS.WritePage(reviewSlug, renderReviewPage(date, review, inputSlugs)); err != nil {
			return Report{}, err
		}
	}
	if s.Reindex != nil {
		if err := s.Reindex(ctx); err != nil {
			return Report{}, err
		}
	}
	return Report{
		RunSlug:     runSlug,
		ReviewSlug:  reviewSlug,
		InputSlugs:  inputSlugs,
		Promoted:    len(promoted),
		ReviewItems: len(review),
		Skipped:     len(parsed.Skipped),
		ModelUsed:   llmResp.Model,
		Warnings:    warnings,
	}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) applyPromotion(p dreamPromotion, pageBySlug map[string]memfs.Page, inputSet map[string]bool, date time.Time) (bool, string) {
	p.TargetSlug = strings.TrimSpace(p.TargetSlug)
	p.Section = normalizeSection(p.Section)
	p.Bullet = normalizeBullet(p.Bullet)
	if p.TargetSlug == "" {
		return false, "missing target_slug"
	}
	page, ok := pageBySlug[p.TargetSlug]
	if !ok {
		return false, "target_slug does not exist"
	}
	if !isCanonicalTarget(p.TargetSlug) {
		return false, "target_slug is not a canonical page"
	}
	if !allowedSection(p.Section) {
		return false, "section is not allowed"
	}
	if p.Bullet == "" || !strings.HasPrefix(p.Bullet, "- ") {
		return false, "bullet must be a markdown list item"
	}
	if len(validSourceSlugs(p.SourceSlugs, inputSet)) == 0 {
		return false, "promotion has no valid source_slugs from this dream input set"
	}
	p.Bullet = ensureSourceCitation(p.Bullet, p.SourceSlugs, inputSet, date)
	if strings.Contains(page.Raw, p.Bullet) {
		return false, "canonical page already contains this bullet"
	}
	updated := appendToSection(page.Raw, p.Section, p.Bullet)
	if err := s.FS.WritePage(p.TargetSlug, updated); err != nil {
		return false, err.Error()
	}
	return true, ""
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
	if len(inputs) > 40 {
		inputs = inputs[:40]
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

func dreamSystemPrompt() string {
	return strings.TrimSpace(`You are jazmem's nightly memory consolidation job.

Extract durable memory candidates from input notes. Be conservative.
Promote only high-confidence facts, preferences, decisions, open loops, and stable relationships that are directly supported by sources.
Do not invent target pages. Do not promote ambiguous claims. Put ambiguous or risky candidates into review.
Every promotion bullet must include a [Source: [[source-slug]], YYYY-MM-DD] citation and must be a single markdown list item.
Use only these sections: Current, Preferences, Decisions, Open Loops, Relationships, Timeline.
Return strict JSON only:
{
  "summary": "brief run summary",
  "promotions": [
    {
      "target_slug": "people/alice",
      "section": "Open Loops",
      "bullet": "- Clarify the Acme follow-up. [Source: [[inbox/acme-note]], 2026-06-08]",
      "confidence": "high",
      "source_slugs": ["inbox/acme-note"]
    }
  ],
  "review": [
    {"candidate": "possible fact", "reason": "why not promoted", "source_slugs": ["inbox/acme-note"]}
  ],
  "skipped": ["noise or non-durable item"]
}`)
}

func dreamUserPrompt(date time.Time, inputs []memfs.Page, canonical []memfs.Page) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Dream date: %s\n\nCanonical pages you may edit:\n", date.Format("2006-01-02"))
	for _, page := range canonical {
		fmt.Fprintf(&b, "- %s — %s\n", page.Slug, page.Title)
	}
	b.WriteString("\nInput pages:\n")
	if len(inputs) == 0 {
		b.WriteString("- No eligible input pages.\n")
		return b.String()
	}
	for _, page := range inputs {
		body := strings.TrimSpace(page.Body)
		if len(body) > 1800 {
			body = body[:1800] + "\n...[truncated]"
		}
		fmt.Fprintf(&b, "\n---\nslug: %s\ntitle: %s\nmodified: %s\nbody:\n%s\n", page.Slug, page.Title, page.ModifiedAt.Format(time.RFC3339), body)
	}
	return b.String()
}

func parseDream(content string) (llmDream, []string) {
	var parsed llmDream
	var warnings []string
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		warnings = append(warnings, "Failed to parse provider dream JSON; raw model output was captured as review.")
		parsed.Review = append(parsed.Review, dreamReview{
			Candidate: content,
			Reason:    "invalid JSON from model",
		})
		return parsed, warnings
	}
	parsed.Summary = strings.TrimSpace(parsed.Summary)
	parsed.Skipped = cleanStrings(parsed.Skipped)
	return parsed, warnings
}

func renderRunPage(date time.Time, inputs []string, parsed llmDream, promoted []dreamPromotion, review []dreamReview, warnings []string, model string) string {
	var b strings.Builder
	b.WriteString(memfs.FrontmatterString(map[string]string{
		"title": "Dream " + date.Format("2006-01-02"),
		"type":  "dream_run",
		"date":  date.Format("2006-01-02"),
	}))
	fmt.Fprintf(&b, "# Dream %s\n\n", date.Format("2006-01-02"))
	fmt.Fprintf(&b, "- Model: `%s`\n", model)
	if parsed.Summary != "" {
		fmt.Fprintf(&b, "- Summary: %s\n", parsed.Summary)
	}
	b.WriteString("\n## Inputs\n\n")
	if len(inputs) == 0 {
		b.WriteString("- No eligible inputs found.\n")
	} else {
		for _, slug := range inputs {
			fmt.Fprintf(&b, "- [[%s]]\n", slug)
		}
	}
	b.WriteString("\n## Promotions\n\n")
	if len(promoted) == 0 {
		b.WriteString("- No canonical page edits were promoted.\n")
	} else {
		for _, item := range promoted {
			fmt.Fprintf(&b, "- [[%s]] / %s: %s\n", item.TargetSlug, item.Section, strings.TrimPrefix(item.Bullet, "- "))
		}
	}
	b.WriteString("\n## Review\n\n")
	if len(review) == 0 {
		b.WriteString("- No review items.\n")
	} else {
		for _, item := range review {
			fmt.Fprintf(&b, "- %s", strings.TrimSpace(item.Candidate))
			if strings.TrimSpace(item.Reason) != "" {
				fmt.Fprintf(&b, " — %s", strings.TrimSpace(item.Reason))
			}
			writeSources(&b, item.SourceSlugs)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n## Skipped\n\n")
	if len(parsed.Skipped) == 0 {
		b.WriteString("- None reported.\n")
	} else {
		for _, item := range parsed.Skipped {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	if len(warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, warning := range warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	return b.String()
}

func renderReviewPage(date time.Time, review []dreamReview, inputs []string) string {
	var b strings.Builder
	b.WriteString(memfs.FrontmatterString(map[string]string{
		"title": "Dream Review " + date.Format("2006-01-02"),
		"type":  "dream_review",
		"date":  date.Format("2006-01-02"),
	}))
	fmt.Fprintf(&b, "# Dream Review %s\n\n", date.Format("2006-01-02"))
	b.WriteString("These candidates were not promoted automatically. Promote by editing canonical markdown directly, then run `jazmem index`.\n\n")
	b.WriteString("## Inputs\n\n")
	for _, slug := range inputs {
		fmt.Fprintf(&b, "- [[%s]]\n", slug)
	}
	b.WriteString("\n## Candidates\n\n")
	for _, item := range review {
		fmt.Fprintf(&b, "- %s", strings.TrimSpace(item.Candidate))
		if strings.TrimSpace(item.Reason) != "" {
			fmt.Fprintf(&b, " — %s", strings.TrimSpace(item.Reason))
		}
		writeSources(&b, item.SourceSlugs)
		b.WriteString("\n")
	}
	return b.String()
}

func appendToSection(raw, section, bullet string) string {
	raw = strings.TrimRight(raw, "\n")
	lines := strings.Split(raw, "\n")
	heading := "## " + section
	start := -1
	end := len(lines)
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i
			continue
		}
		if start >= 0 && i > start && strings.HasPrefix(strings.TrimSpace(line), "## ") {
			end = i
			break
		}
	}
	if start < 0 {
		return raw + "\n\n" + heading + "\n\n" + bullet + "\n"
	}
	insert := []string{bullet}
	if end > start+1 && strings.TrimSpace(lines[end-1]) != "" {
		insert = append([]string{""}, insert...)
	}
	out := append([]string{}, lines[:end]...)
	out = append(out, insert...)
	out = append(out, lines[end:]...)
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

func ensureSourceCitation(bullet string, sourceSlugs []string, inputSet map[string]bool, date time.Time) string {
	if strings.Contains(bullet, "[Source:") {
		return bullet
	}
	valid := validSourceSlugs(sourceSlugs, inputSet)
	if len(valid) == 0 {
		return bullet
	}
	var links []string
	for _, slug := range valid {
		links = append(links, "[["+slug+"]]")
	}
	return strings.TrimRight(bullet, ". ") + ". [Source: " + strings.Join(links, ", ") + ", " + date.Format("2006-01-02") + "]"
}

func normalizeSection(section string) string {
	section = strings.TrimSpace(section)
	for _, allowed := range []string{"Current", "Preferences", "Decisions", "Open Loops", "Relationships", "Timeline"} {
		if strings.EqualFold(section, allowed) {
			return allowed
		}
	}
	return section
}

func normalizeBullet(bullet string) string {
	bullet = strings.TrimSpace(bullet)
	if bullet == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(bullet, "* "); ok {
		bullet = "- " + strings.TrimSpace(after)
	}
	return bullet
}

func allowedSection(section string) bool {
	switch section {
	case "Current", "Preferences", "Decisions", "Open Loops", "Relationships", "Timeline":
		return true
	default:
		return false
	}
}

func isCanonicalTarget(slug string) bool {
	for _, prefix := range []string{"people/", "companies/", "projects/", "concepts/", "notes/"} {
		if strings.HasPrefix(slug, prefix) {
			return true
		}
	}
	return false
}

func canonicalPages(pages []memfs.Page) []memfs.Page {
	var out []memfs.Page
	for _, page := range pages {
		if isCanonicalTarget(page.Slug) {
			out = append(out, page)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func pagesBySlug(pages []memfs.Page) map[string]memfs.Page {
	out := map[string]memfs.Page{}
	for _, page := range pages {
		out[page.Slug] = page
	}
	return out
}

func pageSlugs(pages []memfs.Page) []string {
	out := make([]string, 0, len(pages))
	for _, page := range pages {
		out = append(out, page.Slug)
	}
	return out
}

func slugSet(slugs []string) map[string]bool {
	out := map[string]bool{}
	for _, slug := range slugs {
		out[slug] = true
	}
	return out
}

func validSourceSlugs(slugs []string, inputSet map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" || !inputSet[slug] || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	return out
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func writeSources(b *strings.Builder, slugs []string) {
	slugs = cleanStrings(slugs)
	if len(slugs) == 0 {
		return
	}
	b.WriteString(" [Sources:")
	for _, slug := range slugs {
		fmt.Fprintf(b, " [[%s]]", slug)
	}
	b.WriteString("]")
}
