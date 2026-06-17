package jazmem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRawMarkdownReindexSearchAndDream(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.Local)
	llm := fakeProvider(t, `{"summary":"Promoted durable jazmem search note.","promotions":[{"target_slug":"notes/jazmem-search","section":"Current","bullet":"- Alice and Riley are testing jazmem search. [Source: [[inbox/alice-riley-note]], 2026-06-08]","confidence":"high","source_slugs":["inbox/alice-riley-note"]}],"review":[],"skipped":[]}`)
	defer llm.Close()
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }, APIKey: "test-key", ProviderEndpoint: llm.URL, Model: "test-model", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("inbox/alice-riley-note", "---\ntitle: Alice Riley note\ntype: inbox\n---\n\n# Alice Riley note\n\nAlice and Riley are friends. They are working on jazmem search.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("notes/jazmem-search", "---\ntitle: Jazmem Search\n---\n\n# Jazmem Search\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	results, err := mem.Search(context.Background(), "jazmem search", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatalf("unexpected search results %#v", results)
	}
	foundInbox := false
	for _, result := range results {
		if result.Slug == "inbox/alice-riley-note" {
			foundInbox = true
		}
	}
	if !foundInbox {
		t.Fatalf("inbox note missing from search results %#v", results)
	}

	report, err := mem.Dream(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RunSlug != "dreams/runs/2026-06-08-1200" || len(report.InputSlugs) == 0 || report.Promoted != 1 || report.ModelUsed != "test-model" {
		t.Fatalf("unexpected dream report %#v", report)
	}
	if _, err := mem.GetPage(context.Background(), report.RunSlug); err != nil {
		t.Fatalf("dream run page missing: %v", err)
	}
}

func TestDreamKeepsMultiplePromotionsToSamePage(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.Local)
	llm := fakeProvider(t, `{"summary":"Promoted two notes.","promotions":[{"target_slug":"projects/jazmem","section":"Current","bullet":"- Jazmem keeps markdown as the source of truth. [Source: [[inbox/jazmem-note]], 2026-06-08]","confidence":"high","source_slugs":["inbox/jazmem-note"]},{"target_slug":"projects/jazmem","section":"Open Loops","bullet":"- Jazmem still needs stronger dream consolidation. [Source: [[inbox/jazmem-note]], 2026-06-08]","confidence":"high","source_slugs":["inbox/jazmem-note"]}],"review":[],"skipped":[]}`)
	defer llm.Close()
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }, APIKey: "test-key", ProviderEndpoint: llm.URL, Model: "test-model", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("inbox/jazmem-note", "---\ntitle: Jazmem note\ntype: inbox\n---\n\n# Jazmem note\n\nJazmem keeps markdown as source of truth and needs stronger dream consolidation.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("projects/jazmem", "---\ntitle: Jazmem\n---\n\n# Jazmem\n"); err != nil {
		t.Fatal(err)
	}

	report, err := mem.Dream(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Promoted != 2 {
		t.Fatalf("promoted = %d, want 2; report %#v", report.Promoted, report)
	}
	page, err := mem.GetPage(context.Background(), "projects/jazmem")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Raw, "markdown as the source of truth") || !strings.Contains(page.Raw, "stronger dream consolidation") {
		t.Fatalf("expected both promotions to survive, got:\n%s", page.Raw)
	}
}

func TestDreamUsesConfiguredRunnerAndReindexes(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.Local)
	runner := fakeDreamRunner{
		run: func(_ context.Context, req DreamRequest) (DreamReport, error) {
			if req.Root != root || req.DBPath != dbPath || !req.Date.Equal(now) {
				return DreamReport{}, fmt.Errorf("unexpected dream request %#v", req)
			}
			path := filepath.Join(req.Root, "projects", "agent-dream.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return DreamReport{}, err
			}
			if err := os.WriteFile(path, []byte("---\ntitle: Agent Dream\n---\n\n# Agent Dream\n\nAgent-backed dream wrote this canonical page.\n"), 0o644); err != nil {
				return DreamReport{}, err
			}
			return DreamReport{RunSlug: "dreams/runs/agent-test", ModelUsed: "acp:codex"}, nil
		},
	}
	mem, err := Open(Config{
		Root:        root,
		DBPath:      dbPath,
		Now:         func() time.Time { return now },
		DreamRunner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	report, err := mem.Dream(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RunSlug != "dreams/runs/agent-test" || report.ModelUsed != "acp:codex" {
		t.Fatalf("unexpected report %#v", report)
	}
	results, err := mem.Search(context.Background(), "agent backed dream", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !slugsContain(results, "projects/agent-dream") {
		t.Fatalf("custom dream page was not indexed: %#v", results)
	}
}

func TestDreamReindexesAfterConfiguredRunnerFailure(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	runner := fakeDreamRunner{
		run: func(_ context.Context, req DreamRequest) (DreamReport, error) {
			path := filepath.Join(req.Root, "projects", "partial-dream.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return DreamReport{}, err
			}
			if err := os.WriteFile(path, []byte("---\ntitle: Partial Dream\n---\n\n# Partial Dream\n\nPartial custom dream output should still be searchable.\n"), 0o644); err != nil {
				return DreamReport{}, err
			}
			return DreamReport{RunSlug: "dreams/runs/partial-test", ModelUsed: "acp:codex"}, errors.New("runner stopped after editing")
		},
	}
	mem, err := Open(Config{Root: root, DBPath: dbPath, DreamRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if _, err := mem.Dream(context.Background(), DreamOptions{}); err == nil {
		t.Fatal("expected runner error")
	}
	results, err := mem.Search(context.Background(), "partial custom dream output", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !slugsContain(results, "projects/partial-dream") {
		t.Fatalf("partial custom dream page was not indexed after failure: %#v", results)
	}
}

func TestDreamValidatesHorizonsAfterConfiguredRunner(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	runner := fakeDreamRunner{
		run: func(_ context.Context, req DreamRequest) (DreamReport, error) {
			if err := os.WriteFile(filepath.Join(req.Root, ShortTermFile), []byte("# Wrong\n\n- nope\n"), 0o644); err != nil {
				return DreamReport{}, err
			}
			return DreamReport{RunSlug: "dreams/runs/invalid-horizon", ModelUsed: "acp:codex"}, nil
		},
	}
	mem, err := Open(Config{Root: root, DBPath: dbPath, DreamRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if _, err := mem.Dream(context.Background(), DreamOptions{}); err == nil || !strings.Contains(err.Error(), ShortTermFile) {
		t.Fatalf("expected short-term horizon validation error, got %v", err)
	}
}

func TestDreamAllowsLargeHorizonsAfterConfiguredRunner(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	largeLongTerm := "# Long Term Memory\n\n" + strings.Repeat("- Durable relationship/context detail. [Source: User, chat, 2026-06-10]\n", 80)
	runner := fakeDreamRunner{
		run: func(_ context.Context, req DreamRequest) (DreamReport, error) {
			if err := os.WriteFile(filepath.Join(req.Root, LongTermFile), []byte(largeLongTerm), 0o644); err != nil {
				return DreamReport{}, err
			}
			return DreamReport{RunSlug: "dreams/runs/large-horizon", ModelUsed: "acp:codex"}, nil
		},
	}
	mem, err := Open(Config{Root: root, DBPath: dbPath, DreamRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	report, err := mem.Dream(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatalf("large horizon should not fail validation: %v", err)
	}
	if report.RunSlug != "dreams/runs/large-horizon" {
		t.Fatalf("unexpected dream report %#v", report)
	}
	content, err := mem.ReadHorizonFile(LongTermFile)
	if err != nil {
		t.Fatal(err)
	}
	if content != largeLongTerm {
		t.Fatalf("large horizon changed: %q", content)
	}
}

func TestRunDreamTaskRecordsIndexAndDreamTasks(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.Local)
	runner := fakeDreamRunner{
		run: func(_ context.Context, req DreamRequest) (DreamReport, error) {
			path := filepath.Join(req.Root, "projects", "manual-dream.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return DreamReport{}, err
			}
			if err := os.WriteFile(path, []byte("---\ntitle: Manual Dream\n---\n\n# Manual Dream\n\nManual dream task should be indexed and recorded.\n"), 0o644); err != nil {
				return DreamReport{}, err
			}
			return DreamReport{RunSlug: "dreams/runs/manual", ModelUsed: "acp:codex"}, nil
		},
	}
	mem, err := Open(Config{
		Root:        root,
		DBPath:      dbPath,
		Now:         func() time.Time { return now },
		DreamRunner: runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	report, err := mem.RunDreamTask(context.Background(), DreamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Dream.RunSlug != "dreams/runs/manual" || report.Index.PageCount != 0 {
		t.Fatalf("unexpected manual dream report %#v", report)
	}
	tasks, err := mem.SchedulerStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]TaskStatus{}
	for _, task := range tasks {
		byName[task.Name] = task
	}
	for _, name := range []string{TaskIndexChangedPages, TaskDream} {
		task := byName[name]
		if task.Status != "ok" || !task.LastRunAt.Equal(now) {
			t.Fatalf("%s task not recorded after manual dream: %#v", name, task)
		}
	}
	results, err := mem.Search(context.Background(), "manual dream task", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !slugsContain(results, "projects/manual-dream") {
		t.Fatalf("manual dream page was not indexed: %#v", results)
	}
}

type fakeDreamRunner struct {
	run func(context.Context, DreamRequest) (DreamReport, error)
}

func (f fakeDreamRunner) RunDream(ctx context.Context, req DreamRequest) (DreamReport, error) {
	return f.run(ctx, req)
}

func TestReindexFindsExplicitAndMentionLinks(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	write := func(slug, content string) {
		t.Helper()
		if err := mem.fs.WritePage(slug, content); err != nil {
			t.Fatal(err)
		}
	}
	write("people/alice", "---\ntitle: Alice Smith\naliases: [Alice]\n---\n\n# Alice Smith\n")
	write("people/riley", "---\ntitle: Riley Jones\naliases: [Riley]\n---\n\n# Riley Jones\n")
	write("notes/friendship", "# Friendship\n\n[[people/alice|Alice]] and Riley are friends.\n")

	report, err := mem.Reindex(context.Background(), ReindexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.ExplicitLinks != 1 {
		t.Fatalf("explicit links = %d, want 1", report.ExplicitLinks)
	}
	if report.MentionLinks < 1 {
		t.Fatalf("expected at least one mention link, report %#v", report)
	}
	doctor, err := mem.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doctor.LinkCount < 2 {
		t.Fatalf("expected indexed links, doctor %#v", doctor)
	}
}

func TestSearchFallsBackToBroadTokenMatch(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("projects/ink", "---\ntitle: Ink\n---\n\n# Ink\n\nInk supports enterprise Claude deployment.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("people/majid-yazdani", "---\ntitle: Majid Yazdani\naliases: [Majid]\n---\n\n# Majid Yazdani\n\nMajid is connected to Leeroo strategy.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	results, err := mem.Search(context.Background(), "Ink enterprise Claude deployment Majid Leeroo", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected broad token fallback results, got %#v", results)
	}
	slugs := map[string]bool{}
	for _, result := range results {
		slugs[result.Slug] = true
	}
	if !slugs["projects/ink"] || !slugs["people/majid-yazdani"] {
		t.Fatalf("missing expected slugs from broad results: %#v", results)
	}
}

func TestSearchExpandsExplicitMemlinks(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("people/alice", "---\ntitle: Alice Smith\naliases: [Alice]\n---\n\n# Alice Smith\n\nAlice is friends with [[people/riley]].\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("people/riley", "---\ntitle: Riley Jones\naliases: [Riley]\n---\n\n# Riley Jones\n\nRiley works on memory systems.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	response, err := mem.Retrieve(context.Background(), "Alice", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	slugs := map[string]bool{}
	for _, result := range response.Results {
		slugs[result.Slug] = true
	}
	if !slugs["people/alice"] || !slugs["people/riley"] {
		t.Fatalf("expected direct hit and linked page, got %#v", response.Results)
	}
	if response.Stats.GraphHits < 1 {
		t.Fatalf("expected graph hits, got %#v", response.Stats)
	}
}

func TestTypedRelationshipsDriveRelationalSearch(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	write := func(slug, content string) {
		t.Helper()
		if err := mem.fs.WritePage(slug, content); err != nil {
			t.Fatal(err)
		}
	}
	write("people/alice-example", "---\ntitle: Alice Example\naliases: [Alice]\n---\n\n# Alice Example\n\n## Relationships\n\n- [[companies/widget-co]] - invested in. [Source: User, chat, 2026-06-08]\n- [[companies/acme]] - works at. [Source: User, chat, 2026-06-08]\n- [[people/riley-example]] - friend. [Source: User, chat, 2026-06-08]\n")
	write("people/riley-example", "---\ntitle: Riley Example\naliases: [Riley]\n---\n\n# Riley Example\n")
	write("companies/widget-co", "---\ntitle: Widget Co\naliases: [Widget]\n---\n\n# Widget Co\n\nWidget Co is a company.\n")
	write("companies/acme", "---\ntitle: Acme\n---\n\n# Acme\n\nAcme is a company.\n")

	report, err := mem.Reindex(context.Background(), ReindexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.TypedLinks < 3 {
		t.Fatalf("typed links = %d, want at least 3", report.TypedLinks)
	}
	doctor, err := mem.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doctor.TypedLinkCount < 3 {
		t.Fatalf("typed link count = %d, want at least 3", doctor.TypedLinkCount)
	}

	investors, err := mem.Search(context.Background(), "who invested in Widget Co", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(investors) == 0 || investors[0].Slug != "people/alice-example" {
		t.Fatalf("investor query returned %#v", investors)
	}

	workers, err := mem.Search(context.Background(), "who works at Acme", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(workers) == 0 || workers[0].Slug != "people/alice-example" {
		t.Fatalf("works-at query returned %#v", workers)
	}

	friends, err := mem.Search(context.Background(), "who are Alice's friends", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) == 0 || friends[0].Slug != "people/riley-example" {
		t.Fatalf("friend query returned %#v", friends)
	}

	connection, err := mem.Search(context.Background(), "what connects Alice and Widget Co", SearchOptions{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	connectionSlugs := map[string]bool{}
	for _, result := range connection {
		connectionSlugs[result.Slug] = true
	}
	if !connectionSlugs["people/alice-example"] || !connectionSlugs["companies/widget-co"] {
		t.Fatalf("connection query returned %#v", connection)
	}
}

func TestSearchMaxPoolsChunksBeforePageLimit(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	var noisy strings.Builder
	noisy.WriteString("---\ntitle: Noisy Needle\n---\n\n# Noisy Needle\n\n")
	for i := range 20 {
		fmt.Fprintf(&noisy, "needle appears in noisy chunk %02d.\n\n", i)
	}
	if err := mem.fs.WritePage("projects/noisy-needle", noisy.String()); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("projects/target-needle", "---\ntitle: Target Needle\n---\n\n# Target Needle\n\nneedle appears once in the target page.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	response, err := mem.Retrieve(context.Background(), "needle", SearchOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	slugs := map[string]bool{}
	for _, result := range response.Results {
		slugs[result.Slug] = true
	}
	if !slugs["projects/noisy-needle"] || !slugs["projects/target-needle"] {
		t.Fatalf("expected distinct pages after max-pool, got %#v", response.Results)
	}
}

func TestEvaluateScoresExpectedSlugs(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("projects/ink", "---\ntitle: Ink\n---\n\n# Ink\n\nInk supports enterprise deployment.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("companies/leeroo", "---\ntitle: Leeroo\n---\n\n# Leeroo\n\nLeeroo deployment uses Ink.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	report, err := mem.Evaluate(context.Background(), EvalOptions{
		Limit: 2,
		Cases: []EvalCase{
			{Query: "Leeroo deployment", ExpectedSlugs: []string{"companies/leeroo"}},
			{Query: "Ink enterprise deployment", ExpectedSlugs: []string{"projects/ink"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.CaseCount != 2 || report.HitRate != 1 {
		t.Fatalf("unexpected eval report %#v", report)
	}
}

func TestAgenticSearchReturnsAnswerWithCitations(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	llm := fakeProvider(t, `{"answer":"Leeroo is connected to Ink in the opportunity corpus.","citation_ids":[1],"gaps":[],"warnings":[]}`)
	defer llm.Close()
	mem, err := Open(Config{Root: root, DBPath: dbPath, APIKey: "test-key", ProviderEndpoint: llm.URL, Model: "test-model", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("companies/leeroo", "---\ntitle: Leeroo\naliases: [Leeroo]\n---\n\n# Leeroo\n\nLeeroo is connected to [[projects/ink]] in the opportunity corpus.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("projects/ink", "---\ntitle: Ink\n---\n\n# Ink\n\nInk is a deployment platform.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Reindex(context.Background(), ReindexOptions{}); err != nil {
		t.Fatal(err)
	}

	response, err := mem.AgenticSearch(context.Background(), "Leeroo", AgenticOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Answer, "Leeroo") || len(response.Citations) == 0 {
		t.Fatalf("unexpected agentic response %#v", response)
	}
	if response.Citations[0].Slug != "companies/leeroo" || response.Citations[0].Chunk != 0 {
		t.Fatalf("unexpected citations %#v", response.Citations)
	}
	if response.ModelUsed != "test-model" || !response.SynthesisOK || response.Rounds != 1 {
		t.Fatalf("unexpected synthesis metadata %#v", response)
	}
	if response.Stats.Pages < 2 {
		t.Fatalf("agentic search should ignore caller limit and gather broader context, got stats %#v", response.Stats)
	}
}

func fakeProvider(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected provider path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "test-model" {
			t.Fatalf("model = %#v", payload["model"])
		}
		if payload["reasoning_effort"] != "medium" {
			t.Fatalf("reasoning_effort = %#v", payload["reasoning_effort"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": content}},
			},
		})
	}))
}

func TestLinkHygieneWritesRelationshipReview(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	mem, err := Open(Config{Root: root, DBPath: dbPath, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("people/a", "---\ntitle: A\n---\n\n# A\n\nA and R are friends.\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("people/r", "---\ntitle: R\n---\n\n# R\n"); err != nil {
		t.Fatal(err)
	}

	report, err := mem.LinkHygiene(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.RelationshipsAdded != 0 {
		t.Fatalf("relationships added = %d, want 0", report.RelationshipsAdded)
	}
	if report.ProposalCount != 1 || report.ReviewSlug != "dreams/review/link-hygiene-2026-06-08" {
		t.Fatalf("unexpected hygiene report %#v", report)
	}
	a, err := mem.GetPage(context.Background(), "people/a")
	if err != nil {
		t.Fatal(err)
	}
	r, err := mem.GetPage(context.Background(), "people/r")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a.Raw, "[[people/r]] - friend") {
		t.Fatalf("A page was mutated:\n%s", a.Raw)
	}
	if strings.Contains(r.Raw, "[[people/a]] - friend") {
		t.Fatalf("R page was mutated:\n%s", r.Raw)
	}
	review, err := mem.GetPage(context.Background(), report.ReviewSlug)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(review.Raw, "[[people/r]] - friend") || !strings.Contains(review.Raw, "[[people/a]] - friend") {
		t.Fatalf("review missing proposal bullets:\n%s", review.Raw)
	}
}

func TestGetPageNotFoundSuggestsSimilarSlugs(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	mem, err := Open(Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	if err := mem.fs.WritePage("people/alice-bentick", "---\ntitle: Alice Bentick\naliases: [Alice]\n---\n\n# Alice Bentick\n"); err != nil {
		t.Fatal(err)
	}
	if err := mem.fs.WritePage("people/bob", "---\ntitle: Bob\n---\n\n# Bob\n"); err != nil {
		t.Fatal(err)
	}

	_, err = mem.GetPage(context.Background(), "people/alice")
	if err == nil {
		t.Fatal("expected not found error")
	}
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected NotFoundError, got %T: %v", err, err)
	}
	if len(notFound.Suggestions) == 0 || notFound.Suggestions[0].Slug != "people/alice-bentick" {
		t.Fatalf("unexpected suggestions %#v", notFound.Suggestions)
	}
	if !strings.Contains(err.Error(), "people/alice") {
		t.Fatalf("error did not include requested slug: %v", err)
	}
}
