package jazmem

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetrieveDeepExpandsTwoHops(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("notes/alpha", "---\ntitle: Alpha\n---\n\n# Alpha\n\nQuantum garden research is ongoing. See [[notes/beta]].\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("notes/beta", "---\ntitle: Beta\n---\n\n# Beta\n\nFollow-on work. See [[notes/gamma]].\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("notes/gamma", "---\ntitle: Gamma\n---\n\n# Gamma\n\nSecond hop only content.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	shallow, err := mem.Retrieve(context.Background(), "quantum garden research", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if slugsContain(shallow.Results, "notes/gamma") {
		t.Fatalf("shallow search should not reach two hops, got %#v", shallow.Results)
	}
	if !slugsContain(shallow.Results, "notes/beta") {
		t.Fatalf("shallow search should include one-hop link, got %#v", shallow.Results)
	}

	deep, err := mem.Retrieve(context.Background(), "quantum garden research", SearchOptions{Limit: 5, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slugsContain(deep.Results, "notes/gamma") {
		t.Fatalf("deep search should reach two hops, got %#v", deep.Results)
	}
	for _, result := range deep.Results {
		if result.ModifiedAt.IsZero() {
			t.Fatalf("expected modified_at on results, got %#v", result)
		}
		switch result.Slug {
		case "notes/alpha":
			if result.Via != "" {
				t.Fatalf("direct matches carry no via, got %q", result.Via)
			}
		case "notes/beta", "notes/gamma":
			if result.Via != "link" {
				t.Fatalf("%s should arrive via link, got %q", result.Slug, result.Via)
			}
		}
	}
	if deep.Stats.GraphHits <= shallow.Stats.GraphHits {
		t.Fatalf("deep graph hits %d should exceed shallow %d", deep.Stats.GraphHits, shallow.Stats.GraphHits)
	}

	beta, err := mem.GetPage(context.Background(), "notes/beta")
	if err != nil {
		t.Fatal(err)
	}
	if !linkRefsContain(beta.Links, "notes/gamma") || !linkRefsContain(beta.Backlinks, "notes/alpha") {
		t.Fatalf("expected graph neighborhood on page, got links=%#v backlinks=%#v", beta.Links, beta.Backlinks)
	}
}

func TestAgenticSearchDeepRunsFollowupRound(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	llm := sequencedProvider(t, []string{
		`{"answer":"Leeroo is connected to Ink.","citation_ids":[1],"gaps":["deployment timeline unknown"],"warnings":[],"followup_queries":["Uniforce deployment timeline"]}`,
		`{"answer":"Leeroo is connected to Ink and the Uniforce deployment timeline is six weeks.","citation_ids":[1,3],"gaps":[],"warnings":[]}`,
	})
	defer llm.Close()
	mem, err := Open(Config{Root: root, DBPath: dbPath, APIKey: "test-key", ProviderEndpoint: llm.URL, Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("companies/leeroo", "---\ntitle: Leeroo\n---\n\n# Leeroo\n\nLeeroo is connected to [[projects/ink]] in the opportunity corpus.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("projects/ink", "---\ntitle: Ink\n---\n\n# Ink\n\nInk is a platform.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("concepts/uniforce-deployment-timeline", "---\ntitle: Uniforce Deployment Timeline\n---\n\n# Uniforce Deployment Timeline\n\nUniforce deployment timeline is six weeks.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	response, err := mem.AgenticSearch(context.Background(), "Leeroo", AgenticOptions{Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if response.Rounds != 2 {
		t.Fatalf("expected two synthesis rounds, got %#v", response)
	}
	if response.Diagnostics["followup_queries"] != 1 || response.Diagnostics["followup_chunks"] < 1 {
		t.Fatalf("unexpected followup diagnostics %#v", response.Diagnostics)
	}
	foundFollowup := false
	for _, citation := range response.Citations {
		if citation.Slug == "concepts/uniforce-deployment-timeline" {
			foundFollowup = true
		}
	}
	if !foundFollowup {
		t.Fatalf("expected citation from follow-up retrieval, got %#v", response.Citations)
	}
	if response.Stats.Pages < 3 || response.Stats.Chunks < 3 {
		t.Fatalf("follow-up evidence should be counted in stats, got %#v", response.Stats)
	}
	if !strings.Contains(response.Answer, "six weeks") || !response.SynthesisOK {
		t.Fatalf("unexpected final answer %#v", response)
	}
}

func TestDreamMaintainsHorizonFiles(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 10, 4, 0, 0, 0, time.UTC)
	llm := fakeProvider(t, `{"summary":"horizon refresh","promotions":[],"review":[],"skipped":[],"long_term":"# Long Term Memory\n\n- Goal: $5m through agent products.","short_term":"# Short Term Memory\n\n- Focus: jaz memory system."}`)
	defer llm.Close()
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }, APIKey: "test-key", ProviderEndpoint: llm.URL, Model: "test-model", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	for _, name := range []string{LongTermFile, ShortTermFile} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("Open should create %s skeleton: %v", name, err)
		}
	}
	if err := mem.fs.WritePage("inbox/2026-06-09-goal-note", "---\ntitle: Goal note\ntype: inbox\n---\n\n# Goal note\n\nUser said the ultimate goal is $5m through agent products.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	report, err := mem.Dream(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.LongTermUpdated || !report.ShortTermUpdated {
		t.Fatalf("expected horizon updates, got %#v", report)
	}
	longTerm, err := os.ReadFile(filepath.Join(root, LongTermFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(longTerm), "$5m through agent products") {
		t.Fatalf("unexpected LONG_TERM.md content %q", longTerm)
	}

	results, err := mem.Search(context.Background(), "long term memory", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Slug == "long_term" || result.Slug == "short_term" {
			t.Fatalf("horizon files must not be indexed as pages, got %#v", results)
		}
	}
}

func TestHorizonReadWriteAndSchedulerStatus(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.WriteHorizonFile(LongTermFile, "# Long Term Memory\n\n- Goal: $5m."); err != nil {
		t.Fatal(err)
	}
	content, err := mem.ReadHorizonFile(LongTermFile)
	if err != nil || !strings.Contains(content, "$5m") {
		t.Fatalf("horizon roundtrip failed: %q %v", content, err)
	}
	if err := mem.WriteHorizonFile("AGENTS.md", "nope"); err == nil {
		t.Fatal("non-horizon file write must be rejected")
	}
	shortTermBefore, err := mem.ReadHorizonFile(ShortTermFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteHorizonFile(ShortTermFile, "# Wrong\n\n- nope"); err == nil {
		t.Fatal("horizon content with wrong heading must be rejected")
	}
	shortTermAfter, err := mem.ReadHorizonFile(ShortTermFile)
	if err != nil {
		t.Fatal(err)
	}
	if shortTermAfter != shortTermBefore {
		t.Fatalf("invalid horizon write changed content: %q", shortTermAfter)
	}

	if err := mem.store.RecordTask(context.Background(), "dream", "error", now.Add(-9*time.Hour), "dial tcp: timeout"); err != nil {
		t.Fatal(err)
	}
	tasks, err := mem.SchedulerStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]TaskStatus{}
	for _, task := range tasks {
		byName[task.Name] = task
	}
	if len(tasks) != 6 {
		t.Fatalf("expected all 6 task specs, got %#v", tasks)
	}
	dream := byName["dream"]
	if dream.Status != "error" || dream.Error == "" || dream.LastRunAt.IsZero() {
		t.Fatalf("unexpected dream state %#v", dream)
	}
	if dream.NextDue.IsZero() {
		t.Fatalf("dream next due missing %#v", dream)
	}
	index := byName["index_changed_pages"]
	if !index.LastRunAt.IsZero() || index.NextDue.IsZero() {
		t.Fatalf("never-run task should be due now, got %#v", index)
	}
}

func TestSearchExcludesDreamsLane(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("concepts/quantum-garden", "---\ntitle: Quantum Garden\n---\n\n# Quantum Garden\n\nQuantum garden research notes.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("dreams/review/link-hygiene-2026-06-11", "---\ntitle: Link Hygiene Review\n---\n\n# Review\n\nProposal about quantum garden research and [[concepts/quantum-garden]].\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	results, err := mem.Search(context.Background(), "quantum garden research", SearchOptions{Limit: 10, Deep: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Slug != "concepts/quantum-garden" {
		t.Fatalf("canonical page should rank first, got %#v", results)
	}
	if slugsContain(results, "dreams/review/link-hygiene-2026-06-11") {
		t.Fatalf("dreams/ lane must not appear in search results, got %#v", results)
	}
}

func TestDreamRejectsInvalidHorizonRewrite(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 10, 4, 0, 0, 0, time.UTC)
	llm := fakeProvider(t, `{"summary":"bad horizon","promotions":[],"review":[],"skipped":[],"long_term":"No heading, no write","short_term":""}`)
	defer llm.Close()
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }, APIKey: "test-key", ProviderEndpoint: llm.URL, Model: "test-model", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()
	before, err := mem.ReadHorizonFile(LongTermFile)
	if err != nil {
		t.Fatal(err)
	}

	report, err := mem.Dream(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.LongTermUpdated {
		t.Fatalf("invalid long-term rewrite should not update: %#v", report)
	}
	if !strings.Contains(strings.Join(report.Warnings, "\n"), "must start") {
		t.Fatalf("expected validation warning, got %#v", report.Warnings)
	}
	after, err := mem.ReadHorizonFile(LongTermFile)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("invalid dream rewrite changed LONG_TERM.md: %q", after)
	}
}

func linkRefsContain(refs []LinkRef, slug string) bool {
	for _, ref := range refs {
		if ref.Slug == slug {
			return true
		}
	}
	return false
}

func slugsContain(results []Result, slug string) bool {
	for _, result := range results {
		if result.Slug == slug {
			return true
		}
	}
	return false
}

func sequencedProvider(t *testing.T, contents []string) *httptest.Server {
	t.Helper()
	call := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected provider path %s", r.URL.Path)
		}
		content := contents[len(contents)-1]
		if call < len(contents) {
			content = contents[call]
		}
		call++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": content}},
			},
		})
	}))
}
