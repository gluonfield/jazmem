package jazmem

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/wins/jazmem/internal/llm"
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
	return m.synthesizeAgentic(ctx, query, response)
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

func (m *Memory) synthesizeAgentic(ctx context.Context, query string, response SearchResponse) (AgenticResponse, error) {
	evidence, byID := agenticEvidence(response.Results)
	if len(evidence) == 0 {
		return AgenticResponse{
			Answer:      "No matching memory was found.",
			Gaps:        []string{"No matching markdown pages were found in jazmem."},
			Stats:       response.Stats,
			Rounds:      0,
			SynthesisOK: false,
			Diagnostics: agenticDiagnostics(response),
		}, nil
	}
	llmResp, err := m.llm.CompleteJSON(ctx, llm.Request{
		MaxTokens: 2200,
		Messages: []llm.Message{
			{Role: "system", Content: agenticSystemPrompt()},
			{Role: "user", Content: agenticUserPrompt(query, evidence)},
		},
	})
	if err != nil {
		return AgenticResponse{}, err
	}
	var parsed struct {
		Answer      string   `json:"answer"`
		CitationIDs []int    `json:"citation_ids"`
		Gaps        []string `json:"gaps"`
		Warnings    []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(llmResp.Content), &parsed); err != nil {
		return AgenticResponse{}, fmt.Errorf("decode agentic OpenRouter JSON: %w", err)
	}
	parsed.Answer = strings.TrimSpace(parsed.Answer)
	if parsed.Answer == "" {
		return AgenticResponse{}, fmt.Errorf("agentic OpenRouter response missing answer")
	}
	citations, citationWarnings := citationsFromIDs(parsed.CitationIDs, byID)
	warnings := append([]string{}, parsed.Warnings...)
	warnings = append(warnings, citationWarnings...)
	if len(citations) == 0 {
		warnings = append(warnings, "LLM returned no valid citation ids; answer should be treated as ungrounded.")
	}
	return AgenticResponse{
		Answer:      parsed.Answer,
		Citations:   citations,
		Gaps:        cleanStringSlice(parsed.Gaps),
		Stats:       response.Stats,
		Warnings:    cleanStringSlice(warnings),
		ModelUsed:   llmResp.Model,
		Rounds:      1,
		SynthesisOK: len(citations) > 0,
		Diagnostics: agenticDiagnostics(response),
	}, nil
}

func agenticSystemPrompt() string {
	return strings.TrimSpace(`You are jazmem's memory answerer.

Answer only from the supplied evidence. Do not use outside knowledge.
If the evidence is insufficient, say what is known and put missing information in gaps.
Ground every substantive claim in citation_ids that refer to supplied evidence ids.
Return strict JSON only:
{
  "answer": "concise prose answer",
  "citation_ids": [1, 2],
  "gaps": ["important missing information"],
  "warnings": ["optional retrieval or evidence warnings"]
}`)
}

func agenticUserPrompt(query string, evidence []agenticEvidenceItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n\nEvidence:\n", strings.TrimSpace(query))
	for _, item := range evidence {
		fmt.Fprintf(&b, "\n[%d] slug=%s title=%s chunk=%d score=%.8f\n%s\n", item.ID, item.Citation.Slug, item.Citation.Title, item.Citation.Chunk, item.Score, item.Snippet)
	}
	return b.String()
}

type agenticEvidenceItem struct {
	ID       int
	Citation Citation
	Snippet  string
	Score    float64
}

func agenticEvidence(results []Result) ([]agenticEvidenceItem, map[int]Citation) {
	var evidence []agenticEvidenceItem
	byID := map[int]Citation{}
	id := 1
	for _, result := range results {
		for _, match := range result.Matches {
			snippet := compactSnippet(match.Snippet)
			if snippet == "" {
				continue
			}
			citation := Citation{Slug: result.Slug, Title: result.Title, Chunk: match.Chunk}
			evidence = append(evidence, agenticEvidenceItem{
				ID:       id,
				Citation: citation,
				Snippet:  snippet,
				Score:    match.Score,
			})
			byID[id] = citation
			id++
			if len(evidence) >= 20 {
				return evidence, byID
			}
		}
	}
	return evidence, byID
}

func citationsFromIDs(ids []int, byID map[int]Citation) ([]Citation, []string) {
	var citations []Citation
	var warnings []string
	seen := map[string]bool{}
	for _, id := range ids {
		citation, ok := byID[id]
		if !ok {
			warnings = append(warnings, "LLM returned invalid citation id "+strconv.Itoa(id)+".")
			continue
		}
		key := citation.Slug + "\x00" + strconv.Itoa(citation.Chunk)
		if seen[key] {
			continue
		}
		seen[key] = true
		citations = append(citations, citation)
	}
	return citations, warnings
}

func agenticDiagnostics(response SearchResponse) map[string]int {
	return map[string]int{
		"pages_gathered":  response.Stats.Pages,
		"chunks_gathered": response.Stats.Chunks,
		"graph_hits":      response.Stats.GraphHits,
	}
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

func cleanStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
