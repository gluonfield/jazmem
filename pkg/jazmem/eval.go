package jazmem

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

//go:embed default_eval.json
var defaultEvalJSON []byte

func DefaultEvalCases() ([]EvalCase, error) {
	var cases []EvalCase
	if err := json.Unmarshal(defaultEvalJSON, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func ParseEvalCases(data []byte) ([]EvalCase, error) {
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err == nil {
		return cases, nil
	}
	var wrapped struct {
		Cases []EvalCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Cases, nil
}

func (m *Memory) Evaluate(ctx context.Context, opts EvalOptions) (EvalReport, error) {
	cases := opts.Cases
	if len(cases) == 0 {
		defaults, err := DefaultEvalCases()
		if err != nil {
			return EvalReport{}, err
		}
		cases = defaults
	}
	if len(cases) == 0 {
		return EvalReport{}, errors.New("eval cases are empty")
	}
	limit := normalizeSearchLimit(opts.Limit)
	var results []EvalCaseResult
	var precisionSum, recallSum, rrSum, hitCases float64
	for _, evalCase := range cases {
		caseLimit := limit
		if evalCase.Limit > 0 {
			caseLimit = normalizeSearchLimit(evalCase.Limit)
		}
		search, err := m.Retrieve(ctx, evalCase.Query, SearchOptions{Limit: caseLimit})
		if err != nil {
			return EvalReport{}, err
		}
		returned := returnedSlugs(search.Results, caseLimit)
		caseResult := scoreEvalCase(evalCase, returned)
		results = append(results, caseResult)
		precisionSum += caseResult.Precision
		recallSum += caseResult.Recall
		rrSum += caseResult.ReciprocalRank
		if caseResult.Hits > 0 {
			hitCases++
		}
	}
	n := float64(len(results))
	return EvalReport{
		CaseCount:   len(results),
		Limit:       limit,
		HitRate:     hitCases / n,
		Precision:   precisionSum / n,
		Recall:      recallSum / n,
		MRR:         rrSum / n,
		CaseResults: results,
	}, nil
}

func returnedSlugs(results []Result, limit int) []string {
	out := make([]string, 0, len(results))
	for _, result := range results {
		out = append(out, result.Slug)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func scoreEvalCase(evalCase EvalCase, returned []string) EvalCaseResult {
	expected := normalizeSlugList(evalCase.ExpectedSlugs)
	expectedSet := map[string]bool{}
	for _, slug := range expected {
		expectedSet[slug] = true
	}
	hits := 0
	firstHit := 0
	for i, slug := range returned {
		if expectedSet[slug] {
			hits++
			if firstHit == 0 {
				firstHit = i + 1
			}
		}
	}
	precision := 0.0
	if len(returned) > 0 {
		precision = float64(hits) / float64(len(returned))
	}
	recall := 0.0
	if len(expected) > 0 {
		recall = float64(hits) / float64(len(expected))
	}
	rr := 0.0
	if firstHit > 0 {
		rr = 1 / float64(firstHit)
	}
	return EvalCaseResult{
		Query:          strings.TrimSpace(evalCase.Query),
		ExpectedSlugs:  expected,
		ReturnedSlugs:  returned,
		Hits:           hits,
		Precision:      precision,
		Recall:         recall,
		ReciprocalRank: rr,
	}
}

func normalizeSlugList(slugs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		slug = strings.Trim(strings.TrimSpace(slug), "/")
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}
