package jazmem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskSchemaSeededAndExcluded(t *testing.T) {
	root := t.TempDir()
	mem, err := Open(Config{Root: root, DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	schemaPath := filepath.Join(root, "tasks", "SCHEMA.md")
	if _, err := os.Stat(schemaPath); err != nil {
		t.Fatalf("Open should seed tasks/SCHEMA.md: %v", err)
	}

	// The schema page is documentation, not a task, and must not appear in the list.
	tasks, err := mem.ListTasks(context.Background(), TaskFilter{Status: "all"})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Slug == "tasks/SCHEMA" {
			t.Fatalf("tasks/SCHEMA must be excluded from the task list")
		}
	}
}

func TestListTasksFiltersByStatus(t *testing.T) {
	mem, err := Open(Config{Root: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "index.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mem.Close() }()

	write := func(slug, raw string) {
		t.Helper()
		if err := mem.fs.WritePage(slug, raw); err != nil {
			t.Fatal(err)
		}
	}
	write("projects/jaz", "---\ntitle: Jaz\n---\n\n# Jaz\n\nProject.\n")
	write("tasks/whatsapp", "---\ntitle: WhatsApp pairing\nstatus: in-progress\nproject: projects/jaz\n---\n\n# WhatsApp pairing\n")
	write("tasks/telegram", "---\ntitle: Telegram QR\nstatus: not-started\nproject: projects/jaz\n---\n\n# Telegram QR\n")
	write("tasks/figma", "---\ntitle: Figma OAuth\nstatus: in-progress\n---\n\n# Figma OAuth\n\nBlocked on [[people/irwin]] in the body.\n")
	write("tasks/browser", "---\ntitle: Browser upload\nstatus: done\nproject: projects/jaz\nclosed: 2026-06-20\n---\n\n# Browser upload\n")
	// A task with no status frontmatter defaults to not-started, so it is open.
	write("tasks/loose", "---\ntitle: Loose task\n---\n\n# Loose task\n")

	cases := []struct {
		status string
		want   []string
	}{
		{"open", []string{"tasks/figma", "tasks/whatsapp", "tasks/loose", "tasks/telegram"}},
		{"", []string{"tasks/figma", "tasks/whatsapp", "tasks/loose", "tasks/telegram"}},
		{"all", []string{"tasks/figma", "tasks/whatsapp", "tasks/loose", "tasks/telegram", "tasks/browser"}},
		{"done", []string{"tasks/browser"}},
		{"in-progress", []string{"tasks/figma", "tasks/whatsapp"}},
	}
	for _, tc := range cases {
		tasks, err := mem.ListTasks(context.Background(), TaskFilter{Status: tc.status})
		if err != nil {
			t.Fatalf("status %q: %v", tc.status, err)
		}
		got := make([]string, len(tasks))
		for i, task := range tasks {
			got[i] = task.Slug
		}
		if len(got) != len(tc.want) {
			t.Fatalf("status %q: got %v, want %v", tc.status, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("status %q: order got %v, want %v", tc.status, got, tc.want)
			}
		}
	}

	open, err := mem.ListTasks(context.Background(), TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if open[1].Slug != "tasks/whatsapp" || open[1].Project != "projects/jaz" {
		t.Fatalf("project not parsed from frontmatter: %+v", open[1])
	}
	if open[2].Slug != "tasks/loose" || open[2].Status != "not-started" {
		t.Fatalf("missing status should default to not-started, got %+v", open[2])
	}
}
