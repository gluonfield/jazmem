package dream

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/internal/llm"
	"github.com/gluonfield/jazmem/internal/memfs"
	"github.com/gluonfield/jazmem/internal/templates/dreamprompt"
	"github.com/gluonfield/jazmem/internal/templates/memorypolicy"
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
	RunSlug          string   `json:"run_slug"`
	ReviewSlug       string   `json:"review_slug,omitempty"`
	InputSlugs       []string `json:"input_slugs"`
	Promoted         int      `json:"promoted"`
	ReviewItems      int      `json:"review_items"`
	Skipped          int      `json:"skipped"`
	LongTermUpdated  bool     `json:"long_term_updated,omitempty"`
	ShortTermUpdated bool     `json:"short_term_updated,omitempty"`
	ModelUsed        string   `json:"model_used,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
}

type llmDream struct {
	Summary    string           `json:"summary"`
	Promotions []dreamPromotion `json:"promotions"`
	Review     []dreamReview    `json:"review"`
	Skipped    []string         `json:"skipped"`
	LongTerm   string           `json:"long_term"`
	ShortTerm  string           `json:"short_term"`
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
	longTerm, err := s.FS.ReadRootFile(memfs.LongTermFile)
	if err != nil {
		return Report{}, err
	}
	shortTerm, err := s.FS.ReadRootFile(memfs.ShortTermFile)
	if err != nil {
		return Report{}, err
	}

	systemPrompt, err := dreamSystemPrompt()
	if err != nil {
		return Report{}, err
	}
	userPrompt, err := dreamUserPrompt(date, inputs, canonicalPages(pages), longTerm, shortTerm)
	if err != nil {
		return Report{}, err
	}

	llmResp, err := s.LLM.CompleteJSON(ctx, llm.Request{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	})
	if err != nil {
		return Report{}, err
	}
	parsed, warnings := parseDream(llmResp.Content)
	longTermUpdated, shortTermUpdated, horizonWarnings := s.applyHorizons(parsed)
	warnings = append(warnings, horizonWarnings...)
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

	runSuffix := runSlugSuffix(date)
	runSlug := "dreams/runs/" + runSuffix
	if err := s.FS.WritePage(runSlug, renderRunPage(date, inputSlugs, parsed, promoted, review, warnings, llmResp.Model)); err != nil {
		return Report{}, err
	}
	reviewSlug := ""
	if len(review) > 0 {
		reviewSlug = "dreams/review/dream-" + runSuffix
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
		RunSlug:          runSlug,
		ReviewSlug:       reviewSlug,
		InputSlugs:       inputSlugs,
		Promoted:         len(promoted),
		ReviewItems:      len(review),
		Skipped:          len(parsed.Skipped),
		LongTermUpdated:  longTermUpdated,
		ShortTermUpdated: shortTermUpdated,
		ModelUsed:        llmResp.Model,
		Warnings:         warnings,
	}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func runSlugSuffix(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02-1504")
}

func runLabel(t time.Time) string {
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04")
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
	page.Raw = updated
	pageBySlug[p.TargetSlug] = page
	return true, ""
}

// applyHorizons accepts complete, valid replacements; empty strings leave the
// old horizon file unchanged.
func (s *Service) applyHorizons(parsed llmDream) (longTermUpdated, shortTermUpdated bool, warnings []string) {
	apply := func(name, content string) bool {
		content = strings.TrimSpace(content)
		if content == "" {
			return false
		}
		if err := memfs.ValidateHorizonContent(name, content); err != nil {
			warnings = append(warnings, err.Error()+"; kept previous content")
			return false
		}
		if err := s.FS.WriteRootFile(name, content); err != nil {
			warnings = append(warnings, "write "+name+": "+err.Error())
			return false
		}
		return true
	}
	longTermUpdated = apply(memfs.LongTermFile, parsed.LongTerm)
	shortTermUpdated = apply(memfs.ShortTermFile, parsed.ShortTerm)
	return longTermUpdated, shortTermUpdated, warnings
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

func dreamSystemPrompt() (string, error) {
	return dreamprompt.RenderSystem(dreamprompt.SystemData{
		LongTermPolicy:  memorypolicy.RenderLongTerm(),
		ShortTermPolicy: memorypolicy.RenderShortTerm(),
	})
}

func dreamUserPrompt(date time.Time, inputs []memfs.Page, canonical []memfs.Page, longTerm, shortTerm string) (string, error) {
	canonicalPrompt := make([]dreamprompt.Page, 0, len(canonical))
	for _, page := range canonical {
		canonicalPrompt = append(canonicalPrompt, dreamprompt.Page{
			Slug:  page.Slug,
			Title: page.Title,
		})
	}
	inputPrompt := make([]dreamprompt.Page, 0, len(inputs))
	for _, page := range inputs {
		body := strings.TrimSpace(page.Body)
		if len(body) > 1800 {
			body = body[:1800] + "\n...[truncated]"
		}
		inputPrompt = append(inputPrompt, dreamprompt.Page{
			Slug:       page.Slug,
			Title:      page.Title,
			ModifiedAt: page.ModifiedAt.Format(time.RFC3339),
			Body:       body,
		})
	}
	return dreamprompt.RenderUser(dreamprompt.UserData{
		Date:      date.Format("2006-01-02"),
		LongTerm:  strings.TrimSpace(longTerm),
		ShortTerm: strings.TrimSpace(shortTerm),
		Canonical: canonicalPrompt,
		Inputs:    inputPrompt,
	})
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
	label := runLabel(date)
	b.WriteString(memfs.FrontmatterString(map[string]string{
		"title": "Dream " + label,
		"type":  "dream_run",
		"date":  date.Format("2006-01-02"),
	}))
	fmt.Fprintf(&b, "# Dream %s\n\n", label)
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
	label := runLabel(date)
	b.WriteString(memfs.FrontmatterString(map[string]string{
		"title": "Dream Review " + label,
		"type":  "dream_review",
		"date":  date.Format("2006-01-02"),
	}))
	fmt.Fprintf(&b, "# Dream Review %s\n\n", label)
	b.WriteString("These candidates were not promoted automatically. Promote by editing canonical markdown directly; Jaz owns indexing and maintenance.\n\n")
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
