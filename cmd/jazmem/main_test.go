package main

import "testing"

func TestParseSearchArgsAllowsLimitAfterQuery(t *testing.T) {
	cfg, query, limit, text, err := parseSearchArgs([]string{"Leeroo", "--limit", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root != "" || cfg.DBPath != "" {
		t.Fatalf("unexpected config %#v", cfg)
	}
	if query != "Leeroo" || limit != 1 || text {
		t.Fatalf("query=%q limit=%d text=%v", query, limit, text)
	}
}

func TestParseSearchArgsAllowsLimitBeforeQuery(t *testing.T) {
	_, query, limit, _, err := parseSearchArgs([]string{"--limit=2", "Leeroo"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "Leeroo" || limit != 2 {
		t.Fatalf("query=%q limit=%d", query, limit)
	}
}

func TestParseSearchArgsKeepsLiteralAfterDoubleDash(t *testing.T) {
	_, query, limit, _, err := parseSearchArgs([]string{"--limit", "1", "--", "Leeroo", "--limit", "9"})
	if err != nil {
		t.Fatal(err)
	}
	if query != "Leeroo --limit 9" || limit != 1 {
		t.Fatalf("query=%q limit=%d", query, limit)
	}
}
