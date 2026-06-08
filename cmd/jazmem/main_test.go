package main

import "testing"

func TestParseSearchArgsAllowsLimitAfterQuery(t *testing.T) {
	cfg, query, limit, text, agentic, err := parseSearchArgs([]string{"Leeroo", "--limit", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root != "" || cfg.DBPath != "" {
		t.Fatalf("unexpected config %#v", cfg)
	}
	if query != "Leeroo" || limit != 1 || text || agentic {
		t.Fatalf("query=%q limit=%d text=%v agentic=%v", query, limit, text, agentic)
	}
}

func TestParseSearchArgsAllowsLimitBeforeQuery(t *testing.T) {
	_, query, limit, _, _, err := parseSearchArgs([]string{"--limit=2", "Leeroo"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "Leeroo" || limit != 2 {
		t.Fatalf("query=%q limit=%d", query, limit)
	}
}

func TestParseSearchArgsKeepsLiteralAfterDoubleDash(t *testing.T) {
	_, query, limit, _, _, err := parseSearchArgs([]string{"--limit", "1", "--", "Leeroo", "--limit", "9"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "Leeroo --limit 9" || limit != 1 {
		t.Fatalf("query=%q limit=%d", query, limit)
	}
}

func TestParseSearchArgsAllowsAgenticAfterQuery(t *testing.T) {
	_, query, _, _, agentic, err := parseSearchArgs([]string{"Leeroo", "--agentic"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "Leeroo" || !agentic {
		t.Fatalf("query=%q agentic=%v", query, agentic)
	}
}
