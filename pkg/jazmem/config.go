package jazmem

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultProviderEndpoint = "https://openrouter.ai/api/v1"
	defaultModel            = "openai/gpt-5.4-mini"
)

var loadEnvOnce sync.Once

func DefaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".jaz", "memory")
	}
	return filepath.Join(home, ".jaz", "memory")
}

func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".jaz", "jazmem.sqlite")
	}
	return filepath.Join(home, ".jaz", "jazmem.sqlite")
}

func DefaultDBPathForRoot(root string) string {
	root = cleanPath(root)
	if root == "" || root == cleanPath(DefaultRoot()) {
		return DefaultDBPath()
	}
	return filepath.Join(root, ".jazmem", "index.sqlite")
}

func ResolveConfig(cfg Config) Config {
	loadEnvOnce.Do(loadEnvFiles)

	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("JAZMEM_ROOT"))
	}
	if root == "" {
		root = DefaultRoot()
	}
	root = cleanPath(root)

	dbPath := strings.TrimSpace(cfg.DBPath)
	if dbPath == "" {
		dbPath = strings.TrimSpace(os.Getenv("JAZMEM_DB"))
	}
	if dbPath == "" {
		dbPath = DefaultDBPathForRoot(root)
	}
	cfg.Root = root
	cfg.DBPath = cleanPath(dbPath)

	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = firstEnv("MODEL", "JAZMEM_MODEL")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultModel
	}
	if strings.TrimSpace(cfg.ProviderEndpoint) == "" {
		cfg.ProviderEndpoint = firstEnv("PROVIDER_ENDPOINT", "JAZMEM_PROVIDER_ENDPOINT")
	}
	if strings.TrimSpace(cfg.ProviderEndpoint) == "" {
		cfg.ProviderEndpoint = defaultProviderEndpoint
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		cfg.APIKey = providerAPIKey(cfg.ProviderEndpoint)
	}
	if strings.TrimSpace(cfg.ReasoningEffort) == "" {
		cfg.ReasoningEffort = firstEnv("REASONING_EFFORT", "JAZMEM_REASONING_EFFORT")
	}
	return cfg
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func providerAPIKey(endpoint string) string {
	key := providerAPIKeyEnv(endpoint)
	if key == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(key))
}

func providerAPIKeyEnv(endpoint string) string {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case strings.Contains(endpoint, "openrouter"):
		return "OPENROUTER_API_KEY"
	case strings.Contains(endpoint, "api.openai.com"):
		return "OPENAI_API_KEY"
	default:
		return ""
	}
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if path == "~" {
				path = home
			} else {
				path = filepath.Join(home, path[2:])
			}
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func loadEnvFiles() {
	if os.Getenv("JAZMEM_DISABLE_DOTENV") == "1" {
		return
	}
	for _, path := range envCandidates() {
		loadEnvFile(path)
	}
}

func envCandidates() []string {
	var out []string
	seen := map[string]bool{}
	add := func(path string) {
		path = cleanPath(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, path)
	}
	if explicit := strings.TrimSpace(os.Getenv("JAZMEM_ENV")); explicit != "" {
		add(explicit)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return out
	}
	dir := cwd
	for range 6 {
		add(filepath.Join(dir, ".env"))
		add(filepath.Join(dir, "backend", ".env"))
		add(filepath.Join(dir, "jaz", "backend", ".env"))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return out
}

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if after, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(after)
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
	// Best-effort by design: a torn read behaves like a shorter .env file.
	_ = scanner.Err()
}
