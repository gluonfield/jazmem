package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
)

// backend is the operation surface shared by direct-DB and remote-server
// modes; every CLI command except init and eval runs through it.
type backend interface {
	Retrieve(ctx context.Context, query string, opts jazmem.SearchOptions) (jazmem.SearchResponse, error)
	AgenticSearch(ctx context.Context, query string, opts jazmem.AgenticOptions) (jazmem.AgenticResponse, error)
	GetPage(ctx context.Context, slug string) (jazmem.Page, error)
	ListTasks(ctx context.Context, filter jazmem.TaskFilter) ([]jazmem.Task, error)
	Reindex(ctx context.Context) (jazmem.Report, error)
	Doctor(ctx context.Context) (jazmem.DoctorReport, error)
	Dream(ctx context.Context) (jazmem.DreamReport, error)
	LinkHygiene(ctx context.Context) (jazmem.LinkHygieneReport, error)
	Close() error
}

const serverEnv = "JAZMEM_SERVER"

// Candidates probed when no server is named: jaz's embedded API, then a
// standalone jazmem-server.
var serverCandidates = []string{
	"http://127.0.0.1:5299/jazmem",
	"http://127.0.0.1:9477",
}

// openBackend prefers a running server so the server process stays the single
// writer (no index version skew with the CLI binary). --local forces the
// direct path; --server or JAZMEM_SERVER pins a specific one.
func openBackend(cfg jazmem.Config, server string, forceLocal bool) (backend, error) {
	storage := storageRequestFor(cfg)
	server = strings.TrimSpace(server)
	if forceLocal && server != "" {
		return nil, errors.New("use only one of --server and --local")
	}
	if forceLocal {
		return openLocal(cfg)
	}
	if server == "" {
		server = strings.TrimSpace(os.Getenv(serverEnv))
	}
	if server != "" {
		base := normalizeServerURL(server)
		health, ok := serverHealth(base, 2*time.Second)
		if !ok {
			return nil, fmt.Errorf("jazmem server %s is not reachable", base)
		}
		if err := verifyServerStorage(base, health, storage); err != nil {
			return nil, err
		}
		return newHTTPBackend(base), nil
	}
	if storage.explicit() {
		return openLocal(cfg)
	}
	for _, candidate := range serverCandidates {
		if probeServer(candidate, 250*time.Millisecond) {
			fmt.Fprintf(os.Stderr, "jazmem: using server %s (pass --local for direct database access)\n", candidate)
			return newHTTPBackend(candidate), nil
		}
	}
	return openLocal(cfg)
}

func openLocal(cfg jazmem.Config) (backend, error) {
	memory, err := jazmem.Open(cfg)
	if err != nil {
		return nil, err
	}
	return &localBackend{memory: memory}, nil
}

func normalizeServerURL(server string) string {
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}
	return strings.TrimRight(server, "/")
}

func probeServer(base string, timeout time.Duration) bool {
	_, ok := serverHealth(base, timeout)
	return ok
}

type storageRequest struct {
	rootRequested bool
	dbRequested   bool
	root          string
	dbPath        string
}

func storageRequestFor(cfg jazmem.Config) storageRequest {
	resolved := jazmem.ResolveConfig(cfg)
	return storageRequest{
		rootRequested: strings.TrimSpace(cfg.Root) != "" || strings.TrimSpace(os.Getenv("JAZMEM_ROOT")) != "",
		dbRequested:   strings.TrimSpace(cfg.DBPath) != "" || strings.TrimSpace(os.Getenv("JAZMEM_DB")) != "",
		root:          filepath.Clean(resolved.Root),
		dbPath:        filepath.Clean(resolved.DBPath),
	}
}

func (r storageRequest) explicit() bool {
	return r.rootRequested || r.dbRequested
}

type healthResponse struct {
	Status string `json:"status"`
	Root   string `json:"root"`
	DB     string `json:"db"`
}

func serverHealth(base string, timeout time.Duration) (healthResponse, bool) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(base + "/health")
	if err != nil {
		return healthResponse{}, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return healthResponse{}, false
	}
	var health healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return healthResponse{}, false
	}
	return health, true
}

func verifyServerStorage(base string, health healthResponse, storage storageRequest) error {
	if !storage.explicit() {
		return nil
	}
	var mismatches []string
	if storage.rootRequested && filepath.Clean(health.Root) != storage.root {
		mismatches = append(mismatches, fmt.Sprintf("root %s", health.Root))
	}
	if storage.dbRequested && filepath.Clean(health.DB) != storage.dbPath {
		mismatches = append(mismatches, fmt.Sprintf("db %s", health.DB))
	}
	if len(mismatches) == 0 {
		return nil
	}
	return fmt.Errorf("jazmem server %s uses %s; requested root %s db %s (pass --local for direct database access)", base, strings.Join(mismatches, ", "), storage.root, storage.dbPath)
}

type localBackend struct {
	memory *jazmem.Memory
}

func (b *localBackend) Retrieve(ctx context.Context, query string, opts jazmem.SearchOptions) (jazmem.SearchResponse, error) {
	return b.memory.Retrieve(ctx, query, opts)
}

func (b *localBackend) AgenticSearch(ctx context.Context, query string, opts jazmem.AgenticOptions) (jazmem.AgenticResponse, error) {
	return b.memory.AgenticSearch(ctx, query, opts)
}

func (b *localBackend) GetPage(ctx context.Context, slug string) (jazmem.Page, error) {
	return b.memory.GetPage(ctx, slug)
}

func (b *localBackend) ListTasks(ctx context.Context, filter jazmem.TaskFilter) ([]jazmem.Task, error) {
	return b.memory.ListTasks(ctx, filter)
}

func (b *localBackend) Reindex(ctx context.Context) (jazmem.Report, error) {
	return b.memory.Reindex(ctx, jazmem.ReindexOptions{})
}

func (b *localBackend) Doctor(ctx context.Context) (jazmem.DoctorReport, error) {
	return b.memory.Doctor(ctx)
}

func (b *localBackend) Dream(ctx context.Context) (jazmem.DreamReport, error) {
	return b.memory.Dream(ctx, jazmem.DreamOptions{})
}

func (b *localBackend) LinkHygiene(ctx context.Context) (jazmem.LinkHygieneReport, error) {
	return b.memory.LinkHygiene(ctx)
}

func (b *localBackend) Close() error {
	return b.memory.Close()
}
