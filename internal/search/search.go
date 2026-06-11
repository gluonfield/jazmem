package search

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"

	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
)

type Service struct {
	Store *sqlitestore.Store
}

type Options struct {
	Limit int
	Deep  bool
}

type Response struct {
	Rows      []sqlitestore.SearchResult
	GraphHits int
}

func (s *Service) Search(ctx context.Context, query string, opts Options) (Response, error) {
	limit := normalizeLimit(opts.Limit)
	relationalRows, err := s.relationalSearch(ctx, query, limit)
	if err != nil {
		return Response{}, err
	}
	tagVia(relationalRows, "relationship")
	titleRows, err := s.Store.SearchTitleAlias(ctx, query, limit)
	if err != nil {
		return Response{}, err
	}
	bm25Rows, err := s.Store.Search(ctx, query, chunkCandidateLimit(limit))
	if err != nil {
		return Response{}, err
	}
	merged := mergeRows(relationalRows, titleRows, bm25Rows)
	seeds := topSlugs(merged, limit)
	graphRows, err := s.Store.LinkedPages(ctx, seeds, limit)
	if err != nil {
		return Response{}, err
	}
	graphHits := countNewSlugs(merged, graphRows)
	if opts.Deep {
		hop2Rows, err := s.secondHop(ctx, merged, seeds, graphRows, limit)
		if err != nil {
			return Response{}, err
		}
		graphHits += countNewSlugs(append(append([]sqlitestore.SearchResult{}, merged...), graphRows...), hop2Rows)
		graphRows = append(graphRows, hop2Rows...)
	}
	tagVia(graphRows, "link")
	merged = mergeRows(merged, graphRows)
	candidateCap := chunkCandidateLimit(limit)
	if opts.Deep {
		candidateCap *= 2
	}
	if len(merged) > candidateCap {
		merged = merged[:candidateCap]
	}
	return Response{Rows: merged, GraphHits: graphHits}, nil
}

// secondHop expands links once more from pages the first hop discovered,
// ranked below first-hop results.
func (s *Service) secondHop(ctx context.Context, merged []sqlitestore.SearchResult, seeds []string, graphRows []sqlitestore.SearchResult, limit int) ([]sqlitestore.SearchResult, error) {
	known := map[string]bool{}
	for _, slug := range seeds {
		known[slug] = true
	}
	for _, row := range merged {
		known[row.Slug] = true
	}
	var hop2Seeds []string
	for _, row := range graphRows {
		if known[row.Slug] {
			continue
		}
		known[row.Slug] = true
		hop2Seeds = append(hop2Seeds, row.Slug)
	}
	if len(hop2Seeds) == 0 {
		return nil, nil
	}
	hop2Rows, err := s.Store.LinkedPages(ctx, hop2Seeds, limit)
	if err != nil {
		return nil, err
	}
	out := hop2Rows[:0]
	for _, row := range hop2Rows {
		if known[row.Slug] {
			continue
		}
		row.Score += 0.60
		out = append(out, row)
	}
	return out, nil
}

func (s *Service) relationalSearch(ctx context.Context, query string, limit int) ([]sqlitestore.SearchResult, error) {
	parsed := parseRelationalQuery(query)
	switch parsed.Kind {
	case "fanout":
		seed, err := s.Store.ResolveEntity(ctx, parsed.Seed)
		if err != nil || seed == "" {
			return nil, err
		}
		return s.Store.RelationalFanout(ctx, seed, parsed.LinkTypes, parsed.Direction, limit)
	case "between":
		left, err := s.Store.ResolveEntity(ctx, parsed.Left)
		if err != nil || left == "" {
			return nil, err
		}
		right, err := s.Store.ResolveEntity(ctx, parsed.Right)
		if err != nil || right == "" {
			return nil, err
		}
		return s.Store.RelationalBetween(ctx, left, right, limit)
	default:
		return nil, nil
	}
}

func tagVia(rows []sqlitestore.SearchResult, via string) {
	for i := range rows {
		rows[i].Via = via
	}
}

func mergeRows(groups ...[]sqlitestore.SearchResult) []sqlitestore.SearchResult {
	byMatch := map[string]sqlitestore.SearchResult{}
	for _, group := range groups {
		for _, row := range group {
			key := row.Slug + "\x00" + strconv.Itoa(row.ChunkIndex)
			current, ok := byMatch[key]
			if !ok || row.Score < current.Score {
				byMatch[key] = row
			}
		}
	}
	rows := make([]sqlitestore.SearchResult, 0, len(byMatch))
	for _, row := range byMatch {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(a, b int) bool {
		if rows[a].Score == rows[b].Score {
			if rows[a].Slug == rows[b].Slug {
				return rows[a].ChunkIndex < rows[b].ChunkIndex
			}
			return rows[a].Slug < rows[b].Slug
		}
		return rows[a].Score < rows[b].Score
	})
	return rows
}

func topSlugs(rows []sqlitestore.SearchResult, limit int) []string {
	seen := map[string]bool{}
	slugs := make([]string, 0, limit)
	for _, row := range rows {
		if seen[row.Slug] {
			continue
		}
		seen[row.Slug] = true
		slugs = append(slugs, row.Slug)
		if len(slugs) >= limit {
			break
		}
	}
	return slugs
}

func countNewSlugs(existing, added []sqlitestore.SearchResult) int {
	seen := map[string]bool{}
	for _, row := range existing {
		seen[row.Slug] = true
	}
	count := 0
	for _, row := range added {
		if seen[row.Slug] {
			continue
		}
		seen[row.Slug] = true
		count++
	}
	return count
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return min(limit, 50)
}

func chunkCandidateLimit(pageLimit int) int {
	return min(max(pageLimit*4, 10), 50)
}

type relationalQuery struct {
	Kind      string
	Seed      string
	Left      string
	Right     string
	LinkTypes []string
	Direction string
}

type relationalPattern struct {
	RE        *regexp.Regexp
	LinkTypes []string
	Direction string
}

var betweenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*what\s+connects\s+(.+?)\s+and\s+(.+?)\s*\??\s*$`),
	regexp.MustCompile(`(?i)^\s*how\s+(?:is|are)\s+(.+?)\s+connected\s+to\s+(.+?)\s*\??\s*$`),
}

var fanoutPatterns = []relationalPattern{
	{regexp.MustCompile(`(?i)^\s*who\s+(?:has\s+)?(?:invested|invests)\s+in\s+(.+?)\s*\??\s*$`), []string{"invested_in"}, "in"},
	{regexp.MustCompile(`(?i)^\s*who\s+(?:works|worked)\s+(?:at|for)\s+(.+?)\s*\??\s*$`), []string{"works_at"}, "in"},
	{regexp.MustCompile(`(?i)^\s*who\s+(?:is|was)\s+(?:the\s+)?(?:director|head)\s+of\s+(.+?)\s*\??\s*$`), []string{"works_at"}, "in"},
	{regexp.MustCompile(`(?i)^\s*who\s+(?:founded|started|co-founded|cofounded)\s+(.+?)\s*\??\s*$`), []string{"founder_of"}, "in"},
	{regexp.MustCompile(`(?i)^\s*who\s+(?:advises|advised|is\s+(?:an\s+)?advisor\s+to)\s+(.+?)\s*\??\s*$`), []string{"advises"}, "in"},
	{regexp.MustCompile(`(?i)^\s*what\s+(?:companies|projects|orgs|organizations)\s+(?:has|does|did)\s+(.+?)\s+(?:invest(?:ed)?\s+in|invests\s+in)\s*\??\s*$`), []string{"invested_in"}, "out"},
	{regexp.MustCompile(`(?i)^\s*what\s+(?:companies|projects|orgs|organizations)\s+(?:has|does|did)\s+(.+?)\s+(?:advise|advises|advised)\s*\??\s*$`), []string{"advises"}, "out"},
	{regexp.MustCompile(`(?i)^\s*where\s+(?:does|did)\s+(.+?)\s+(?:work|works|worked)\s*\??\s*$`), []string{"works_at"}, "out"},
	{regexp.MustCompile(`(?i)^\s*who\s+(?:are|is)\s+(.+?)'?s\s+friends\s*\??\s*$`), []string{"friend"}, "both"},
	{regexp.MustCompile(`(?i)^\s*who\s+(?:is|are)\s+friends\s+with\s+(.+?)\s*\??\s*$`), []string{"friend"}, "both"},
	{regexp.MustCompile(`(?i)^\s*who\s+(?:works|worked)\s+with\s+(.+?)\s*\??\s*$`), []string{"works_with"}, "both"},
}

func parseRelationalQuery(query string) relationalQuery {
	query = strings.TrimSpace(query)
	for _, re := range betweenPatterns {
		if match := re.FindStringSubmatch(query); len(match) == 3 {
			return relationalQuery{
				Kind:  "between",
				Left:  cleanEntity(match[1]),
				Right: cleanEntity(match[2]),
			}
		}
	}
	for _, pattern := range fanoutPatterns {
		if match := pattern.RE.FindStringSubmatch(query); len(match) == 2 {
			return relationalQuery{
				Kind:      "fanout",
				Seed:      cleanEntity(match[1]),
				LinkTypes: pattern.LinkTypes,
				Direction: pattern.Direction,
			}
		}
	}
	return relationalQuery{}
}

func cleanEntity(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " \t\r\n?.!,;:\"'`")
	for _, prefix := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(strings.ToLower(value), prefix) && len(value) > len(prefix) {
			return strings.TrimSpace(value[len(prefix):])
		}
	}
	return value
}
