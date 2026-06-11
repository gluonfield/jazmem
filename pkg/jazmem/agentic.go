package jazmem

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gluonfield/jazmem/internal/llm"
)

type agenticParsed struct {
	Answer          string   `json:"answer"`
	CitationIDs     []int    `json:"citation_ids"`
	Gaps            []string `json:"gaps"`
	Warnings        []string `json:"warnings"`
	FollowupQueries []string `json:"followup_queries"`
}

type agenticEvidenceItem struct {
	ID       int
	Citation Citation
	Snippet  string
	Score    float64
}

// evidencePool numbers retrieved chunks for citation and dedupes across
// retrieval rounds. IDs are stable once assigned, so round-two answers can
// cite round-one evidence.
type evidencePool struct {
	items     []agenticEvidenceItem
	byID      map[int]Citation
	seen      map[string]bool
	seenPages map[string]bool
	limit     int
}

type evidenceAdd struct {
	Pages  int
	Chunks int
}

func newEvidencePool(limit int) *evidencePool {
	return &evidencePool{byID: map[int]Citation{}, seen: map[string]bool{}, seenPages: map[string]bool{}, limit: limit}
}

// Add appends unseen chunks as numbered evidence and reports how many it took.
func (p *evidencePool) Add(results []Result) evidenceAdd {
	var added evidenceAdd
	for _, result := range results {
		for _, match := range result.Matches {
			if len(p.items) >= p.limit {
				return added
			}
			key := result.Slug + "\x00" + strconv.Itoa(match.Chunk)
			if p.seen[key] {
				continue
			}
			snippet := compactSnippet(match.Snippet)
			if snippet == "" {
				continue
			}
			p.seen[key] = true
			id := len(p.items) + 1
			citation := Citation{ID: id, Slug: result.Slug, Title: result.Title, Chunk: match.Chunk}
			p.items = append(p.items, agenticEvidenceItem{ID: id, Citation: citation, Snippet: snippet, Score: match.Score})
			p.byID[id] = citation
			added.Chunks++
			if !p.seenPages[result.Slug] {
				p.seenPages[result.Slug] = true
				added.Pages++
			}
		}
	}
	return added
}

func (m *Memory) synthesizeAgentic(ctx context.Context, query string, response SearchResponse, deep bool, evidenceCap int) (AgenticResponse, error) {
	pool := newEvidencePool(evidenceCap)
	pool.Add(response.Results)
	stats := response.Stats
	diagnostics := agenticDiagnostics(stats)
	if len(pool.items) == 0 {
		return AgenticResponse{
			Answer:      "No matching memory was found.",
			Gaps:        []string{"No matching markdown pages were found in jazmem."},
			Stats:       response.Stats,
			Rounds:      0,
			SynthesisOK: false,
			Diagnostics: diagnostics,
		}, nil
	}
	parsed, llmResp, err := m.completeAgentic(ctx, query, pool.items)
	if err != nil {
		return AgenticResponse{}, err
	}
	rounds := 1
	var warnings []string
	if deep {
		followups := cleanFollowupQueries(parsed.FollowupQueries, query)
		var added evidenceAdd
		for _, followup := range followups {
			followupResp, err := m.retrieve(ctx, followup, agenticFollowupPageLimit, false)
			if err != nil {
				warnings = append(warnings, "follow-up retrieval failed: "+followup)
				continue
			}
			next := pool.Add(followupResp.Results)
			added.Pages += next.Pages
			added.Chunks += next.Chunks
			stats.GraphHits += followupResp.Stats.GraphHits
		}
		stats.Pages += added.Pages
		stats.Chunks += added.Chunks
		diagnostics = agenticDiagnostics(stats)
		diagnostics["followup_queries"] = len(followups)
		diagnostics["followup_pages"] = added.Pages
		diagnostics["followup_chunks"] = added.Chunks
		if added.Chunks > 0 {
			secondParsed, secondResp, err := m.completeAgentic(ctx, query, pool.items)
			if err != nil {
				warnings = append(warnings, "second synthesis round failed; answer uses round one evidence only")
			} else {
				parsed, llmResp = secondParsed, secondResp
				rounds = 2
			}
		}
	}
	citations, citationWarnings := citationsFromIDs(parsed.CitationIDs, pool.byID)
	warnings = append(warnings, parsed.Warnings...)
	warnings = append(warnings, citationWarnings...)
	if len(citations) == 0 {
		warnings = append(warnings, "LLM returned no valid citation ids; answer should be treated as ungrounded.")
	}
	return AgenticResponse{
		Answer:      parsed.Answer,
		Citations:   citations,
		Gaps:        cleanStringSlice(parsed.Gaps),
		Stats:       stats,
		Warnings:    cleanStringSlice(warnings),
		ModelUsed:   llmResp.Model,
		Rounds:      rounds,
		SynthesisOK: len(citations) > 0,
		Diagnostics: diagnostics,
	}, nil
}

func (m *Memory) completeAgentic(ctx context.Context, query string, evidence []agenticEvidenceItem) (agenticParsed, llm.Response, error) {
	llmResp, err := m.llm.CompleteJSON(ctx, llm.Request{
		MaxTokens: 2200,
		Messages: []llm.Message{
			{Role: "system", Content: agenticSystemPrompt()},
			{Role: "user", Content: agenticUserPrompt(query, evidence)},
		},
	})
	if err != nil {
		return agenticParsed{}, llm.Response{}, err
	}
	var parsed agenticParsed
	if err := json.Unmarshal([]byte(llmResp.Content), &parsed); err != nil {
		return agenticParsed{}, llm.Response{}, fmt.Errorf("decode agentic provider JSON: %w", err)
	}
	parsed.Answer = strings.TrimSpace(parsed.Answer)
	if parsed.Answer == "" {
		return agenticParsed{}, llm.Response{}, fmt.Errorf("agentic provider response missing answer")
	}
	return parsed, llmResp, nil
}

func cleanFollowupQueries(queries []string, original string) []string {
	originalKey := strings.ToLower(strings.Join(strings.Fields(original), " "))
	seen := map[string]bool{originalKey: true}
	var out []string
	for _, query := range queries {
		query = strings.TrimSpace(query)
		key := strings.ToLower(strings.Join(strings.Fields(query), " "))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, query)
		if len(out) >= agenticFollowupQueryLimit {
			break
		}
	}
	return out
}

func agenticSystemPrompt() string {
	return strings.TrimSpace(`You are jazmem's memory answerer.

Answer only from the supplied evidence. Do not use outside knowledge.
If the evidence is insufficient, say what is known and put missing information in gaps.
Ground every substantive claim in citation_ids that refer to supplied evidence ids.
When gaps remain, propose up to 3 followup_queries built from concrete nouns and names likely to retrieve the missing evidence; they are executed only when deep retrieval is enabled.
Return strict JSON only:
{
  "answer": "concise prose answer",
  "citation_ids": [1, 2],
  "gaps": ["important missing information"],
  "warnings": ["optional retrieval or evidence warnings"],
  "followup_queries": ["optional search queries to fill gaps"]
}`)
}

func agenticUserPrompt(query string, evidence []agenticEvidenceItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n\nEvidence:\n", strings.TrimSpace(query))
	for _, item := range evidence {
		fmt.Fprintf(&b, "\n[%d] slug=%s title=%s chunk=%d\n%s\n", item.ID, item.Citation.Slug, item.Citation.Title, item.Citation.Chunk, item.Snippet)
	}
	return b.String()
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

func agenticDiagnostics(stats SearchStats) map[string]int {
	return map[string]int{
		"pages_gathered":  stats.Pages,
		"chunks_gathered": stats.Chunks,
		"graph_hits":      stats.GraphHits,
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
	const maxSnippet = 900
	if len(snippet) <= maxSnippet {
		return snippet
	}
	cut := maxSnippet
	if boundary := strings.LastIndexAny(snippet[:maxSnippet], ".!?]"); boundary >= 400 {
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
		if after, ok := strings.CutPrefix(trimmed, "- "); ok {
			snippet = strings.TrimSpace(after)
			continue
		}
		if after, ok := strings.CutPrefix(trimmed, "* "); ok {
			snippet = strings.TrimSpace(after)
			continue
		}
		return trimmed
	}
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
