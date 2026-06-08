package jazmem

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jazmem/internal/search"
	sqlitestore "github.com/wins/jazmem/internal/store/sqlite"
)

func (m *Memory) Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
	response, err := m.retrieve(ctx, query, opts.Limit)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

func (m *Memory) Retrieve(ctx context.Context, query string, opts SearchOptions) (SearchResponse, error) {
	return m.retrieve(ctx, query, opts.Limit)
}

func (m *Memory) AgenticSearch(ctx context.Context, query string, opts AgenticOptions) (AgenticResponse, error) {
	response, err := m.retrieve(ctx, query, opts.Limit)
	if err != nil {
		return AgenticResponse{}, err
	}
	return buildAgenticResponse(response), nil
}

func (m *Memory) retrieve(ctx context.Context, query string, rawLimit int) (SearchResponse, error) {
	limit := normalizeSearchLimit(rawLimit)
	candidates, err := m.search.Search(ctx, query, search.Options{Limit: limit})
	if err != nil {
		return SearchResponse{}, err
	}
	results := mergeChunkResults(candidates.Rows, limit)
	chunks := 0
	for _, result := range results {
		chunks += len(result.Matches)
	}
	return SearchResponse{
		Results: results,
		Stats: SearchStats{
			Pages:     len(results),
			Chunks:    chunks,
			GraphHits: candidates.GraphHits,
		},
	}, nil
}

func RenderSearchText(response SearchResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Results: %d pages, %d matched chunks\n\n", response.Stats.Pages, response.Stats.Chunks)
	if len(response.Results) == 0 {
		b.WriteString("No matching memory chunks were found.\n")
		return b.String()
	}
	for i, result := range response.Results {
		fmt.Fprintf(&b, "[%d] %s (%s, %.8f)\n", i+1, result.Title, result.Slug, result.Score)
		for _, match := range result.Matches {
			if strings.TrimSpace(match.Snippet) == "" {
				continue
			}
			fmt.Fprintf(&b, "  chunk %d: %s\n", match.Chunk, strings.TrimSpace(match.Snippet))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func RenderAgenticText(response AgenticResponse) string {
	var b strings.Builder
	b.WriteString(response.Answer)
	if !strings.HasSuffix(response.Answer, "\n") {
		b.WriteString("\n")
	}
	if len(response.Gaps) > 0 {
		b.WriteString("\nGaps:\n")
		for _, gap := range response.Gaps {
			fmt.Fprintf(&b, "- %s\n", gap)
		}
	}
	return b.String()
}

func mergeChunkResults(rows []sqlitestore.SearchResult, limit int) []Result {
	results := make([]Result, 0, limit)
	bySlug := map[string]int{}
	for _, row := range rows {
		idx, ok := bySlug[row.Slug]
		if !ok {
			if len(results) >= limit {
				continue
			}
			idx = len(results)
			bySlug[row.Slug] = idx
			results = append(results, Result{
				Slug:  row.Slug,
				Title: row.Title,
				Score: row.Score,
			})
		}
		result := &results[idx]
		if len(result.Matches) == 0 || row.Score < result.Score {
			result.Score = row.Score
		}
		result.Matches = append(result.Matches, Match{
			Chunk:   row.ChunkIndex,
			Snippet: row.Snippet,
			Score:   row.Score,
		})
	}
	return results
}

func buildAgenticResponse(response SearchResponse) AgenticResponse {
	answer, citations := synthesizeExtractiveAnswer(response.Results)
	gaps := []string(nil)
	if len(response.Results) == 0 {
		gaps = append(gaps, "No matching markdown pages were found in jazmem.")
	}
	return AgenticResponse{
		Answer:    answer,
		Citations: citations,
		Gaps:      gaps,
		Stats:     response.Stats,
	}
}

func synthesizeExtractiveAnswer(results []Result) (string, []Citation) {
	if len(results) == 0 {
		return "No matching memory was found.", nil
	}
	var b strings.Builder
	b.WriteString("Most relevant memory:\n")
	citations := make([]Citation, 0)
	seenCitations := map[string]bool{}
	for _, result := range results {
		wrotePage := false
		for _, match := range result.Matches {
			snippet := compactSnippet(match.Snippet)
			if snippet == "" {
				continue
			}
			if !wrotePage {
				fmt.Fprintf(&b, "\n%s (%s):\n", result.Title, result.Slug)
				wrotePage = true
			}
			ref := fmt.Sprintf("[Source: [[%s]], chunk %d]", result.Slug, match.Chunk)
			fmt.Fprintf(&b, "- %s %s\n", snippet, ref)
			key := result.Slug + "\x00" + string(rune(match.Chunk))
			if !seenCitations[key] {
				seenCitations[key] = true
				citations = append(citations, Citation{
					Slug:  result.Slug,
					Title: result.Title,
					Chunk: match.Chunk,
				})
			}
		}
	}
	if len(citations) == 0 {
		return "Matching pages were found, but no usable snippets were available.", nil
	}
	return strings.TrimSpace(b.String()), citations
}

func compactSnippet(snippet string) string {
	snippet = stripMarkdownHeadings(snippet)
	snippet = strings.TrimSpace(snippet)
	if snippet == "" {
		return ""
	}
	snippet = strings.Join(strings.Fields(snippet), " ")
	snippet = trimLeadingListMarker(snippet)
	const maxSnippet = 560
	if len(snippet) <= maxSnippet {
		return snippet
	}
	cut := maxSnippet
	if boundary := strings.LastIndexAny(snippet[:maxSnippet], ".!?]"); boundary >= 240 {
		cut = boundary + 1
	}
	return strings.TrimSpace(snippet[:cut]) + "..."
}

func stripMarkdownHeadings(snippet string) string {
	lines := strings.Split(snippet, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, " ")
}

func trimLeadingListMarker(snippet string) string {
	for {
		trimmed := strings.TrimSpace(snippet)
		if strings.HasPrefix(trimmed, "- ") {
			snippet = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			continue
		}
		if strings.HasPrefix(trimmed, "* ") {
			snippet = strings.TrimSpace(strings.TrimPrefix(trimmed, "* "))
			continue
		}
		return trimmed
	}
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}
