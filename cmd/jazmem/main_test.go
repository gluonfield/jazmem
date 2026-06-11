package main

import "testing"

func TestParseSearchArgsAllowsLimitAfterQuery(t *testing.T) {
	parsed, err := parseSearchArgs([]string{"Leeroo", "--limit", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.cfg.Root != "" || parsed.cfg.DBPath != "" {
		t.Fatalf("unexpected config %#v", parsed.cfg)
	}
	if parsed.query != "Leeroo" || parsed.limit != 1 || parsed.text || parsed.agentic || parsed.deep {
		t.Fatalf("unexpected args %#v", parsed)
	}
}

func TestParseSearchArgsAllowsLimitBeforeQuery(t *testing.T) {
	parsed, err := parseSearchArgs([]string{"--limit=2", "Leeroo"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.query != "Leeroo" || parsed.limit != 2 {
		t.Fatalf("query=%q limit=%d", parsed.query, parsed.limit)
	}
}

func TestParseSearchArgsKeepsLiteralAfterDoubleDash(t *testing.T) {
	parsed, err := parseSearchArgs([]string{"--limit", "1", "--", "Leeroo", "--limit", "9"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.query != "Leeroo --limit 9" || parsed.limit != 1 {
		t.Fatalf("query=%q limit=%d", parsed.query, parsed.limit)
	}
}

func TestParseSearchArgsAllowsAgenticAfterQuery(t *testing.T) {
	parsed, err := parseSearchArgs([]string{"Leeroo", "--agentic"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.query != "Leeroo" || !parsed.agentic {
		t.Fatalf("query=%q agentic=%v", parsed.query, parsed.agentic)
	}
}

func TestParseSearchArgsAllowsDeep(t *testing.T) {
	parsed, err := parseSearchArgs([]string{"--agentic", "--deep", "Leeroo"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.query != "Leeroo" || !parsed.agentic || !parsed.deep {
		t.Fatalf("unexpected args %#v", parsed)
	}
}
