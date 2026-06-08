package jazmem

import (
	"os"
	"path/filepath"
	"strings"
)

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
	return cfg
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
