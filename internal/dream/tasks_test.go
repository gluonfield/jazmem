package dream

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/internal/memfs"
)

func TestArchiveDoneTasks(t *testing.T) {
	dir := t.TempDir()
	fs := memfs.New(dir)
	s := &Service{FS: fs}

	taskRaw := "---\ntitle: WhatsApp pairing\nstatus: done\nproject: projects/jaz\nclosed: 2026-06-20\n---\n\n# WhatsApp pairing\n\nDetails.\n"
	if err := fs.WritePage("projects/jaz", "---\ntitle: Jaz\n---\n\n# Jaz\n\n## State\n\n- Building.\n"); err != nil {
		t.Fatal(err)
	}
	if err := fs.WritePage("tasks/whatsapp", taskRaw); err != nil {
		t.Fatal(err)
	}
	// An open task must never be archived.
	if err := fs.WritePage("tasks/telegram", "---\ntitle: Telegram QR\nstatus: in-progress\nproject: projects/jaz\n---\n\n# Telegram QR\n"); err != nil {
		t.Fatal(err)
	}

	date := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	archived, warnings, err := s.ArchiveDoneTasks(context.Background(), date)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if archived != 1 {
		t.Fatalf("archived = %d, want 1", archived)
	}

	project, err := fs.ReadPage("projects/jaz")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(project.Raw, "## Completed Tasks") {
		t.Fatalf("project page missing Completed Tasks section:\n%s", project.Raw)
	}
	if !strings.Contains(project.Raw, "[[tasks/whatsapp]] — WhatsApp pairing. Closed 2026-06-20.") {
		t.Fatalf("project page missing clean task residue:\n%s", project.Raw)
	}
	if strings.Contains(project.Raw, "[[tasks/telegram]]") {
		t.Fatalf("open task should not be archived:\n%s", project.Raw)
	}

	// The task file itself is never rewritten or deleted.
	task, err := fs.ReadPage("tasks/whatsapp")
	if err != nil {
		t.Fatal(err)
	}
	if task.Raw != taskRaw {
		t.Fatalf("task file was modified:\n%s", task.Raw)
	}

	// Running again is a no-op: the residue is already present.
	archived, _, err = s.ArchiveDoneTasks(context.Background(), date)
	if err != nil {
		t.Fatal(err)
	}
	if archived != 0 {
		t.Fatalf("second run archived = %d, want 0 (idempotent)", archived)
	}
}
