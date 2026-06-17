package dream

import (
	"strings"
	"testing"
)

func TestDreamSystemPromptIncludesLongTermPromotionBar(t *testing.T) {
	prompt, err := dreamSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"LONG_TERM.md is profile memory",
		"routine coding style",
		"feature decisions",
		"weak one-off contacts",
		"SHORT_TERM.md is the active working set",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("dream prompt missing %q:\n%s", want, prompt)
		}
	}
}
