package jazmem

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/internal/memfs"
)

// TaskLane is the flat lane that holds tracked work. A task is any page whose
// slug lives here; status lives in frontmatter, never in the path, so a task
// keeps its slug and graph edges across its whole lifecycle.
const TaskLane = "tasks"

const defaultTaskStatus = "not-started"

// Task is a tracked unit of work materialized from a page in the tasks/ lane.
type Task struct {
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Project    string    `json:"project,omitempty"`
	Opened     string    `json:"opened,omitempty"`
	Closed     string    `json:"closed,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
}

// TaskFilter selects which tasks ListTasks returns. Status is "open" (anything
// not done, the default), "done", "all", or an exact status value.
type TaskFilter struct {
	Status string `json:"status,omitempty"`
}

// ListTasks reads the tasks/ lane straight from markdown and filters by status.
// It never touches the index: a task list is a structured enumeration, not a
// ranked search, so it stays correct even before the next reindex.
func (m *Memory) ListTasks(ctx context.Context, filter TaskFilter) ([]Task, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	pages, err := m.fs.ListPages()
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.TrimSpace(filter.Status))
	if want == "" {
		want = "open"
	}
	var tasks []Task
	for _, page := range pages {
		if !strings.HasPrefix(page.Slug, TaskLane+"/") {
			continue
		}
		task := taskFromPage(page)
		if !taskMatchesStatus(task.Status, want) {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(a, b int) bool {
		if ra, rb := statusRank(tasks[a].Status), statusRank(tasks[b].Status); ra != rb {
			return ra < rb
		}
		return tasks[a].Slug < tasks[b].Slug
	})
	return tasks, nil
}

func taskFromPage(page memfs.Page) Task {
	status := strings.ToLower(page.Field("status"))
	if status == "" {
		status = defaultTaskStatus
	}
	return Task{
		Slug:       page.Slug,
		Title:      page.Title,
		Status:     status,
		Project:    memfs.RefSlug(page.Field("project")),
		Opened:     page.Field("opened"),
		Closed:     page.Field("closed"),
		ModifiedAt: page.ModifiedAt,
	}
}

func taskMatchesStatus(status, want string) bool {
	switch want {
	case "all":
		return true
	case "open":
		return status != "done"
	default:
		return status == want
	}
}

// statusRank surfaces active work first: in-progress, then not-started, then
// any custom status, then done.
func statusRank(status string) int {
	switch status {
	case "in-progress":
		return 0
	case "not-started":
		return 1
	case "done":
		return 3
	default:
		return 2
	}
}

// taskSchemaDoc is seeded into tasks/SCHEMA.md so the lane documents itself. It
// is the single source of truth for the task shape; keep it in step with
// taskFromPage, statusRank, and ArchiveDoneTasks.
const taskSchemaDoc = "# Tasks — schema\n" + `
One markdown page per tracked task, flat in this folder. Status is a frontmatter
field, never a folder: a task keeps its slug and links for its whole life, and the
working view is a status filter, not a directory. Nothing is deleted — a done task
stays here as the record.

There is no ` + "`type`" + ` field; the ` + "`tasks/`" + ` folder is the type.

## Frontmatter

- ` + "`title`" + ` — short imperative name. Required.
- ` + "`status`" + ` — ` + "`not-started`" + ` · ` + "`in-progress`" + ` · ` + "`done`" + `. Required; defaults to not-started.
- ` + "`project`" + ` — slug of the project this advances, e.g. ` + "`projects/jaz`" + `. Optional; it
  rides the link graph, so a project's backlinks show its open work.
- ` + "`opened`" + ` — YYYY-MM-DD. Optional.
- ` + "`closed`" + ` — YYYY-MM-DD, set when status becomes done. Optional.

Blockers, who you're waiting on, links, and notes go in the body, not in
frontmatter.

## Shape

` + "```md" + `
---
title: WhatsApp QR password continuation
status: in-progress
project: projects/jaz
opened: 2026-06-27
closed:
---

# WhatsApp QR password continuation

What it is and the next concrete action. Blocked on [[people/irwin]] replying — say
so here in the body.

## Log
- 2026-06-27 opened
` + "```" + `

## Lifecycle

- Create: add ` + "`tasks/<slug>.md`" + ` with status not-started or in-progress.
- Work: flip ` + "`status`" + ` in place. The slug never moves.
- Done: set ` + "`status: done`" + ` and the ` + "`closed`" + ` date. Dream rolls a one-line residue
  onto the project page's ` + "`## Completed Tasks`" + ` and leaves this file as the record.

## Listing

- ` + "`jazmem tasks`" + ` — open tasks (anything not done); ` + "`--status done|all|<status>`" + `.
  This is how the human reads the working set.
- Agents read a known task by its exact slug and create or advance one by editing its
  markdown. Don't enumerate tasks through memory search — it ranks, it doesn't list.

This page is generated by jazmem and excluded from search and the task list.
`
