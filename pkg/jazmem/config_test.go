package jazmem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigUsesDefaultJazPaths(t *testing.T) {
	t.Setenv("JAZMEM_ROOT", "")
	t.Setenv("JAZMEM_DB", "")
	cfg := ResolveConfig(Config{})
	if cfg.Root != cleanPath(DefaultRoot()) {
		t.Fatalf("root = %q, want %q", cfg.Root, cleanPath(DefaultRoot()))
	}
	wantSuffix := filepath.Join(".jaz", "memory")
	if filepath.ToSlash(cfg.Root) == "" || filepath.Base(cfg.Root) != "memory" || !endsWithPath(cfg.Root, wantSuffix) {
		t.Fatalf("default root = %q, want suffix %q", cfg.Root, wantSuffix)
	}
	if cfg.DBPath != cleanPath(DefaultDBPath()) {
		t.Fatalf("db = %q, want %q", cfg.DBPath, cleanPath(DefaultDBPath()))
	}
}

func TestResolveConfigDerivesDBForCustomRoot(t *testing.T) {
	t.Setenv("JAZMEM_ROOT", "")
	t.Setenv("JAZMEM_DB", "")
	root := filepath.Join(t.TempDir(), "mem")
	cfg := ResolveConfig(Config{Root: root})
	want := filepath.Join(cleanPath(root), ".jazmem", "index.sqlite")
	if cfg.DBPath != want {
		t.Fatalf("db = %q, want %q", cfg.DBPath, want)
	}
}

func TestResolveConfigUsesEnvOverrides(t *testing.T) {
	root := filepath.Join(t.TempDir(), "env-mem")
	db := filepath.Join(t.TempDir(), "env.sqlite")
	t.Setenv("JAZMEM_ROOT", root)
	t.Setenv("JAZMEM_DB", db)
	cfg := ResolveConfig(Config{})
	if cfg.Root != cleanPath(root) {
		t.Fatalf("root = %q, want %q", cfg.Root, cleanPath(root))
	}
	if cfg.DBPath != cleanPath(db) {
		t.Fatalf("db = %q, want %q", cfg.DBPath, cleanPath(db))
	}
}

func TestResolveConfigUsesProviderEnv(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("MODEL", "test-model")
	t.Setenv("PROVIDER_ENDPOINT", "https://provider.example/v1")
	t.Setenv("REASONING_EFFORT", "medium")

	cfg := ResolveConfig(Config{})
	if cfg.APIKey != "" {
		t.Fatalf("api key = %q, want empty for unrecognized provider endpoint", cfg.APIKey)
	}
	if cfg.Model != "test-model" {
		t.Fatalf("model = %q, want test-model", cfg.Model)
	}
	if cfg.ProviderEndpoint != "https://provider.example/v1" {
		t.Fatalf("provider endpoint = %q", cfg.ProviderEndpoint)
	}
	if cfg.ReasoningEffort != "medium" {
		t.Fatalf("reasoning effort = %q, want medium", cfg.ReasoningEffort)
	}
}

func TestResolveConfigUsesOpenRouterAPIKeyForOpenRouterEndpoint(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")

	cfg := ResolveConfig(Config{})
	if cfg.APIKey != "openrouter-key" {
		t.Fatalf("api key = %q, want openrouter-key", cfg.APIKey)
	}
}

func TestResolveConfigUsesOpenAIAPIKeyForOpenAIEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-key")

	cfg := ResolveConfig(Config{ProviderEndpoint: "https://api.openai.com/v1"})
	if cfg.APIKey != "openai-key" {
		t.Fatalf("api key = %q, want openai-key", cfg.APIKey)
	}
}

func TestInitBootstrapsLayoutAndIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "memory")
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	report, err := Init(t.Context(), Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	if report.Root != cleanPath(root) || report.DBPath != cleanPath(dbPath) {
		t.Fatalf("unexpected paths %#v", report)
	}
	if !report.MarkdownSourceOfTruth {
		t.Fatalf("markdown source of truth flag was false")
	}
	for _, dir := range report.Directories {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			t.Fatalf("missing bootstrap dir %s: %v", dir, err)
		}
	}
	if len(report.CreatedDirs) == 0 {
		t.Fatalf("expected created dirs in first init report %#v", report)
	}
	second, err := Init(t.Context(), Config{Root: root, DBPath: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.CreatedDirs) != 0 || len(second.ExistingDirs) != len(second.Directories) {
		t.Fatalf("init should be idempotent, got %#v", second)
	}
}

func endsWithPath(path, suffix string) bool {
	rel, err := filepath.Rel(filepath.Dir(filepath.Dir(path)), path)
	if err != nil {
		return false
	}
	return filepath.ToSlash(rel) == filepath.ToSlash(suffix)
}
