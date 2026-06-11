package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gluonfield/jazmem/internal/httpapi"
	"github.com/gluonfield/jazmem/pkg/jazmem"
)

func testServerBackend(t *testing.T) backend {
	t.Helper()
	clearBackendEnv(t)
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })
	if err := memory.WriteHorizonFile(jazmem.LongTermFile, "# Long Term Memory\n\n- Goal: $5m."); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(memory))
	t.Cleanup(server.Close)

	b, err := openBackend(jazmem.Config{}, server.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, ok := b.(*httpBackend); !ok {
		t.Fatalf("explicit server should pick http backend, got %T", b)
	}
	return b
}

func TestHTTPBackendRoundtrip(t *testing.T) {
	b := testServerBackend(t)
	ctx := context.Background()

	report, err := b.Reindex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.PageCount != 0 {
		t.Fatalf("fresh root should have no pages, got %#v", report)
	}

	doctor, err := b.Doctor(ctx)
	if err != nil || doctor.Root == "" {
		t.Fatalf("doctor failed: %#v %v", doctor, err)
	}

	if _, err := b.Retrieve(ctx, "anything", jazmem.SearchOptions{Limit: 5}); err != nil {
		t.Fatalf("remote search failed: %v", err)
	}

	_, err = b.GetPage(ctx, "people/missing")
	var notFound *jazmem.NotFoundError
	if !errors.As(err, &notFound) || notFound.Slug != "people/missing" {
		t.Fatalf("remote miss should map to NotFoundError, got %v", err)
	}
}

func TestOpenBackendRejectsConflictingFlags(t *testing.T) {
	clearBackendEnv(t)
	if _, err := openBackend(jazmem.Config{}, "http://example.com", true); err == nil {
		t.Fatal("server + local must conflict")
	}
}

func TestOpenBackendUnreachableServerErrors(t *testing.T) {
	clearBackendEnv(t)
	if _, err := openBackend(jazmem.Config{}, "http://127.0.0.1:1", false); err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("unreachable server should error early, got %v", err)
	}
}

func TestOpenBackendSkipsAutoServerForExplicitStorage(t *testing.T) {
	clearBackendEnv(t)
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })
	server := httptest.NewServer(httpapi.New(memory))
	t.Cleanup(server.Close)
	origCandidates := serverCandidates
	serverCandidates = []string{server.URL}
	t.Cleanup(func() { serverCandidates = origCandidates })

	b, err := openBackend(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "local.sqlite")}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, ok := b.(*localBackend); !ok {
		t.Fatalf("explicit storage should skip auto server, got %T", b)
	}
}

func TestOpenBackendRejectsExplicitServerStorageMismatch(t *testing.T) {
	clearBackendEnv(t)
	memory, err := jazmem.Open(jazmem.Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = memory.Close() })
	server := httptest.NewServer(httpapi.New(memory))
	t.Cleanup(server.Close)

	_, err = openBackend(jazmem.Config{Root: t.TempDir()}, server.URL, false)
	if err == nil || !strings.Contains(err.Error(), "requested root") {
		t.Fatalf("storage mismatch should error, got %v", err)
	}
}

func clearBackendEnv(t *testing.T) {
	t.Helper()
	t.Setenv(serverEnv, "")
	t.Setenv("JAZMEM_ROOT", "")
	t.Setenv("JAZMEM_DB", "")
}
