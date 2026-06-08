package memgit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type EnsureReport struct {
	RepoPath         string `json:"repo_path"`
	Initialized      bool   `json:"initialized"`
	GitignoreUpdated bool   `json:"gitignore_updated"`
}

type CheckpointReport struct {
	RepoPath   string `json:"repo_path"`
	Committed  bool   `json:"committed"`
	Commit     string `json:"commit,omitempty"`
	Message    string `json:"message"`
	FilesAdded int    `json:"files_added"`
}

func Ensure(ctx context.Context, root string) (EnsureReport, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return EnsureReport{}, errors.New("memory root is empty")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return EnsureReport{}, err
	}
	report := EnsureReport{RepoPath: root}
	repoRoot, err := currentRepoRoot(ctx, root)
	if err != nil || repoRoot != root {
		if _, err := run(ctx, root, "init"); err != nil {
			return EnsureReport{}, err
		}
		report.Initialized = true
	}
	if err := ensureLocalConfig(ctx, root, "user.name", "jazmem"); err != nil {
		return EnsureReport{}, err
	}
	if err := ensureLocalConfig(ctx, root, "user.email", "jazmem@local"); err != nil {
		return EnsureReport{}, err
	}
	updated, err := ensureGitignore(root)
	if err != nil {
		return EnsureReport{}, err
	}
	report.GitignoreUpdated = updated
	return report, nil
}

func Checkpoint(ctx context.Context, root, message string) (CheckpointReport, error) {
	ensure, err := Ensure(ctx, root)
	if err != nil {
		return CheckpointReport{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "jazmem checkpoint"
	}
	if _, err := run(ctx, ensure.RepoPath, "add", "-A", "--", "."); err != nil {
		return CheckpointReport{}, err
	}
	status, err := run(ctx, ensure.RepoPath, "status", "--porcelain")
	if err != nil {
		return CheckpointReport{}, err
	}
	report := CheckpointReport{
		RepoPath:   ensure.RepoPath,
		Message:    message,
		FilesAdded: countChangedFiles(status),
	}
	if strings.TrimSpace(status) == "" {
		if commit, err := currentCommit(ctx, ensure.RepoPath); err == nil {
			report.Commit = commit
		}
		return report, nil
	}
	if _, err := run(ctx, ensure.RepoPath, "commit", "-m", message); err != nil {
		return CheckpointReport{}, err
	}
	commit, err := currentCommit(ctx, ensure.RepoPath)
	if err != nil {
		return CheckpointReport{}, err
	}
	report.Committed = true
	report.Commit = commit
	return report, nil
}

func ensureLocalConfig(ctx context.Context, root, key, value string) error {
	out, err := run(ctx, root, "config", "--local", "--get", key)
	if err == nil && strings.TrimSpace(out) != "" {
		return nil
	}
	_, err = run(ctx, root, "config", "--local", key, value)
	return err
}

func ensureGitignore(root string) (bool, error) {
	path := filepath.Join(root, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	lines := strings.Split(string(existing), "\n")
	seen := map[string]bool{}
	for _, line := range lines {
		seen[strings.TrimSpace(line)] = true
	}
	var additions []string
	for _, pattern := range []string{
		".jazmem/",
		"*.sqlite",
		"*.sqlite-shm",
		"*.sqlite-wal",
		".DS_Store",
	} {
		if !seen[pattern] {
			additions = append(additions, pattern)
		}
	}
	if len(additions) == 0 {
		return false, nil
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(string(existing), "\n"))
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	for _, pattern := range additions {
		b.WriteString(pattern)
		b.WriteString("\n")
	}
	return true, os.WriteFile(path, []byte(b.String()), 0o644)
}

func currentCommit(ctx context.Context, root string) (string, error) {
	out, err := run(ctx, root, "rev-parse", "--short", "HEAD")
	return strings.TrimSpace(out), err
}

func currentRepoRoot(ctx context.Context, root string) (string, error) {
	out, err := run(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(out)), nil
}

func countChangedFiles(status string) int {
	count := 0
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func run(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
