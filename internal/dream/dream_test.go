package dream

import (
	"strings"
	"testing"
)

func TestDreamSystemPromptIncludesLongTermPromotionBar(t *testing.T) {
	prompt := dreamSystemPrompt()
	for _, want := range []string{
		"LONG_TERM.md is profile-level memory",
		"routine coding style preferences",
		"feature decisions",
		"one-off meeting",
		"Short-term horizon policy",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("dream prompt missing %q:\n%s", want, prompt)
		}
	}
}
