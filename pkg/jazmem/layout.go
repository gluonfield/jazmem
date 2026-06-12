package jazmem

import (
	"os"
	"path/filepath"

	"github.com/gluonfield/jazmem/internal/memfs"
)

type LayoutReport struct {
	Root                string   `json:"root"`
	DBPath              string   `json:"db_path"`
	Directories         []string `json:"directories"`
	CreatedDirs         []string `json:"created_dirs"`
	ExistingDirs        []string `json:"existing_dirs"`
	CreatedHorizonFiles []string `json:"created_horizon_files,omitempty"`
}

func EnsureLayout(cfg Config) (LayoutReport, error) {
	cfg = ResolveConfig(cfg)
	return ensureLayoutResolved(cfg)
}

func ensureLayoutResolved(cfg Config) (LayoutReport, error) {
	fs := memfs.New(cfg.Root)
	horizons, err := fs.EnsureHorizonFiles()
	if err != nil {
		return LayoutReport{}, err
	}
	layout, err := fs.EnsureLayoutReport()
	if err != nil {
		return LayoutReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return LayoutReport{}, err
	}
	return LayoutReport{
		Root:                cfg.Root,
		DBPath:              cfg.DBPath,
		Directories:         layout.Directories,
		CreatedDirs:         layout.Created,
		ExistingDirs:        layout.Existing,
		CreatedHorizonFiles: horizons,
	}, nil
}
