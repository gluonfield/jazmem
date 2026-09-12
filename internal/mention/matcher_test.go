package mention

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMatcherPreservesRegexpBoundariesAndCase(t *testing.T) {
	aliases := []string{"Alice", "Alice Smith", "Smith", "KELVIN", "longs", "Σigma", "café", "C++", "a.b", "foo_bar", "Alice", "a b", "ab"}
	matcher := New(aliases)
	bodies := []string{
		"ALICE Smith and Alice. KELVIN Kelvin longſ ΣIGMA σigma ςigma café CAFÉ C++ a.b foo_bar",
		"xAlice Alice2 _Alice Alice_ éAlice KAlice ſAlice Aliceé AliceK Aliceſ",
		"Alice", "ab ab", "aa b a b", "a.b.c C++foo C++!", "no matches", "", "\xffAlice\xff",
		strings.Repeat("prefix ", 100) + "Alice Smith " + strings.Repeat("suffix ", 100),
	}
	for _, body := range bodies {
		matches, err := matcher.Find(t.Context(), body)
		if err != nil {
			t.Fatal(err)
		}
		byAlias := map[int]Match{}
		for _, match := range matches {
			byAlias[match.Alias] = match
		}
		for index, alias := range aliases {
			want := regexp.MustCompile(`(?i)(^|[^[:alnum:]_])` + regexp.QuoteMeta(alias) + `([^[:alnum:]_]|$)`).FindStringIndex(body)
			got, found := byAlias[index]
			if found != (want != nil) || found && (got.Start != want[0] || got.End != want[1]) {
				t.Errorf("body=%q alias=%q got=%+v found=%v want=%v", body, alias, got, found, want)
			}
		}
	}
}

func FuzzMatcherMatchesRegexp(f *testing.F) {
	for _, seed := range [][2]string{{"Alice", "éALICE! Alice_"}, {"Kelvin", "kelvin KELVIN"}, {"C++", "C++!"}, {"a b", "a a b"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, alias, body string) {
		if alias == "" || len(alias) > 128 || len(body) > 8192 || !utf8.ValidString(alias) {
			return
		}
		want := regexp.MustCompile(`(?i)(^|[^[:alnum:]_])` + regexp.QuoteMeta(alias) + `([^[:alnum:]_]|$)`).FindStringIndex(body)
		matches, err := New([]string{alias}).Find(t.Context(), body)
		if err != nil {
			t.Fatal(err)
		}
		if (len(matches) > 0) != (want != nil) || len(matches) > 0 && (matches[0].Start != want[0] || matches[0].End != want[1]) {
			t.Fatalf("matches=%v want=%v", matches, want)
		}
	})
}

func TestMatcherCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := New([]string{"Alice"}).Find(ctx, strings.Repeat("Alice ", 10000))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

type cancelDuringScan struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *cancelDuringScan) Err() error {
	c.checks++
	if c.checks == 3 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestMatcherChecksCancellationDuringScan(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	probe := &cancelDuringScan{Context: ctx, cancel: cancel}
	_, err := New([]string{"Alice"}).Find(probe, strings.Repeat("unrelated ", 1000))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("scan completed without checking cancellation: %v", err)
	}
}

func BenchmarkMatcher(b *testing.B) {
	aliases := []string{"Alice Smith", "Bob Jones", "Oxford", "Jaz", "physics", "engineering"}
	matcher := New(aliases)
	body := strings.Repeat("Alice Smith works in engineering at Oxford. ", 1000)
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		if _, err := matcher.Find(b.Context(), body); err != nil {
			b.Fatal(err)
		}
	}
}
