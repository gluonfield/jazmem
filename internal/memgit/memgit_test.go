package memgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCreatesRepoAtMemoryRootAndCheckpointCommits(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	report, err := Ensure(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Initialized {
		t.Fatalf("expected repo initialization, got %#v", report)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("missing .git: %v", err)
	}
	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitignore), ".jazmem/") || !strings.Contains(string(gitignore), "*.sqlite") {
		t.Fatalf("gitignore missing jazmem patterns:\n%s", gitignore)
	}

	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := Checkpoint(t.Context(), root, "initial memory")
	if err != nil {
		t.Fatal(err)
	}
	if !checkpoint.Committed || checkpoint.Commit == "" || checkpoint.FilesAdded < 1 {
		t.Fatalf("unexpected checkpoint %#v", checkpoint)
	}
	clean, err := Checkpoint(t.Context(), root, "no changes")
	if err != nil {
		t.Fatal(err)
	}
	if clean.Committed {
		t.Fatalf("expected no commit for clean repo, got %#v", clean)
	}
}

func TestEnsureCreatesNestedRepoAtMemoryRoot(t *testing.T) {
	requireGit(t)

	parent := t.TempDir()
	if out, err := exec.Command("git", "-C", parent, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init parent: %v\n%s", err, out)
	}
	root := filepath.Join(parent, "memory")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := Ensure(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Initialized {
		t.Fatalf("expected nested memory repo initialization, got %#v", report)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("missing nested .git: %v", err)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}
