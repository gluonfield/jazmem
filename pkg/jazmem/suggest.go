package jazmem

import (
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/gluonfield/jazmem/internal/memfs"
)

type SlugSuggestion struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type NotFoundError struct {
	Slug        string           `json:"slug"`
	Suggestions []SlugSuggestion `json:"suggestions"`
}

func (e *NotFoundError) Error() string {
	return "not found: " + e.Slug
}

func (m *Memory) notFoundError(slug string, err error) error {
	if !os.IsNotExist(err) {
		return err
	}
	return &NotFoundError{Slug: memfs.CleanSlug(slug), Suggestions: m.suggestSlugs(slug, 8)}
}

func (m *Memory) suggestSlugs(input string, limit int) []SlugSuggestion {
	input = memfs.CleanSlug(input)
	pages, err := m.fs.ListPages()
	if err != nil {
		return nil
	}
	type scored struct {
		page  memfs.Page
		score int
	}
	scoredPages := make([]scored, 0, len(pages))
	for _, page := range pages {
		score := slugScore(input, page.Slug, page.Title, page.Aliases)
		if score <= 0 {
			continue
		}
		scoredPages = append(scoredPages, scored{page: page, score: score})
	}
	sort.Slice(scoredPages, func(i, j int) bool {
		if scoredPages[i].score == scoredPages[j].score {
			return scoredPages[i].page.Slug < scoredPages[j].page.Slug
		}
		return scoredPages[i].score > scoredPages[j].score
	})
	if limit <= 0 || limit > len(scoredPages) {
		limit = len(scoredPages)
	}
	out := make([]SlugSuggestion, 0, limit)
	for _, item := range scoredPages[:limit] {
		out = append(out, SlugSuggestion{
			Slug:  item.page.Slug,
			Title: item.page.Title,
		})
	}
	return out
}

func slugScore(input, slug, title string, aliases []string) int {
	input = strings.ToLower(memfs.CleanSlug(input))
	slug = strings.ToLower(memfs.CleanSlug(slug))
	if input == "" || slug == "" {
		return 0
	}
	inputDir, inputTail := splitSlug(input)
	slugDir, _ := splitSlug(slug)
	text := suggestionText(slug, title, aliases)
	inputTokens := regexTokens(input)
	tailTokens := regexTokens(inputTail)
	score := 0
	if inputDir != "" && inputDir == slugDir {
		score += 50
	}
	if slug == input {
		score += 500
	}
	if regexPhrase(input).MatchString(text) {
		score += 220
	}
	if inputTail != "" && regexPhrase(inputTail).MatchString(text) {
		score += 180
	}
	for _, token := range inputTokens {
		if regexToken(token).MatchString(text) {
			score += 45
		}
	}
	for _, token := range tailTokens {
		if regexToken(token).MatchString(text) {
			score += 90
			continue
		}
		if regexPrefixToken(token).MatchString(text) {
			score += 60
		}
	}
	if strings.HasPrefix(slug, input) {
		score += 220
	}
	return score
}

func suggestionText(slug, title string, aliases []string) string {
	parts := []string{slug, title}
	parts = append(parts, aliases...)
	text := strings.ToLower(strings.Join(parts, " "))
	return slugSeparatorsRE.ReplaceAllString(text, " ")
}

func regexTokens(value string) []string {
	value = slugSeparatorsRE.ReplaceAllString(strings.ToLower(value), " ")
	raw := strings.Fields(value)
	out := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

func regexPhrase(value string) *regexp.Regexp {
	tokens := regexTokens(value)
	if len(tokens) == 0 {
		return regexp.MustCompile(`a^`)
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, regexp.QuoteMeta(token))
	}
	return regexp.MustCompile(`(^|[^[:alnum:]])` + strings.Join(parts, `[^[:alnum:]]+`) + `([^[:alnum:]]|$)`)
}

func regexToken(token string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^[:alnum:]])` + regexp.QuoteMeta(token) + `([^[:alnum:]]|$)`)
}

func regexPrefixToken(token string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^[:alnum:]])` + regexp.QuoteMeta(token) + `[[:alnum:]]*([^[:alnum:]]|$)`)
}

func splitSlug(slug string) (string, string) {
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		return slug[:i], slug[i+1:]
	}
	return "", slug
}

var slugSeparatorsRE = regexp.MustCompile(`[^[:alnum:]]+`)
