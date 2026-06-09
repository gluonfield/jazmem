package sqlite

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().UnixMilli()
}

var ftsToken = regexp.MustCompile(`[A-Za-z0-9_]+`)

var searchStopwords = map[string]bool{
	"about": true,
	"and":   true,
	"are":   true,
	"for":   true,
	"how":   true,
	"the":   true,
	"what":  true,
	"where": true,
	"which": true,
	"who":   true,
	"why":   true,
	"with":  true,
	"from":  true,
	"into":  true,
	"is":    true,
	"me":    true,
	"my":    true,
	"tell":  true,
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func chunkPoolLimit(pageLimit int) int {
	limit := normalizeLimit(pageLimit) * 8
	if limit < 50 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func ftsTokens(query string) []string {
	tokens := ftsToken.FindAllString(query, -1)
	if len(tokens) > 8 {
		tokens = tokens[:8]
	}
	return tokens
}

func lookupTerms(query string) []string {
	full := normalizeLookup(query)
	tokens := ftsTokens(query)
	seen := map[string]bool{}
	var terms []string
	add := func(term string) {
		term = normalizeLookup(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	add(full)
	for _, token := range tokens {
		term := normalizeLookup(token)
		if searchStopwords[term] {
			continue
		}
		if len(term) < 2 && len(tokens) > 1 {
			continue
		}
		add(term)
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	return terms
}

func normalizeLookup(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func normalizeEntity(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Trim(s, " \t\r\n?.!,;:\"'`")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	return strings.Join(strings.Fields(s), " ")
}

func cleanEntityPhrase(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t\r\n?.!,;:\"'`")
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(strings.ToLower(s), prefix) && len(s) > len(prefix) {
			return strings.TrimSpace(s[len(prefix):])
		}
	}
	return s
}

func cleanSlug(s string) string {
	s = filepath.ToSlash(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".md")
	s = strings.Trim(s, " \t\r\n?.!,;:\"'`")
	s = strings.Trim(s, "/")
	if strings.Contains(s, "..") {
		return ""
	}
	return strings.ToLower(s)
}

func ftsQueryAll(tokens []string) string {
	return ftsQueryJoin(tokens, " AND ")
}

func ftsQueryAny(tokens []string) string {
	if len(tokens) <= 1 {
		return ""
	}
	return ftsQueryJoin(tokens, " OR ")
}

func ftsQueryJoin(tokens []string, separator string) string {
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.ReplaceAll(token, `"`, `""`)
		parts = append(parts, fmt.Sprintf(`"%s"`, token))
	}
	return strings.Join(parts, separator)
}

func isFTSQuerySyntaxError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "fts5") &&
		(strings.Contains(text, "syntax") || strings.Contains(text, "malformed") || strings.Contains(text, "unterminated"))
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
