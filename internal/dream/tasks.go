package dream

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gluonfield/jazmem/internal/memfs"
)

const taskArchiveSection = "Completed Tasks"

// ArchiveDoneTasks records every done task as a one-line residue on its linked
// project page, leaving the task file itself untouched. Nothing is deleted: the
// task keeps living in the tasks/ lane with status: done, and the project page
// gains a durable, human-readable rollup. It is idempotent — a task whose link
// already appears on its project page is skipped — and reindexing is the
// caller's responsibility.
func (s *Service) ArchiveDoneTasks(ctx context.Context, date time.Time) (int, []string, error) {
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	default:
	}
	pages, err := s.FS.ListPages()
	if err != nil {
		return 0, nil, err
	}
	pageBySlug := pagesBySlug(pages)
	slugs := make([]string, 0, len(pageBySlug))
	for slug := range pageBySlug {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	var warnings []string
	archived := 0
	for _, slug := range slugs {
		page := pageBySlug[slug]
		if !strings.HasPrefix(page.Slug, "tasks/") {
			continue
		}
		if strings.ToLower(page.Field("status")) != "done" {
			continue
		}
		projectSlug := memfs.RefSlug(page.Field("project"))
		if projectSlug == "" {
			continue
		}
		project, ok := pageBySlug[projectSlug]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("task %s references missing project %s", slug, projectSlug))
			continue
		}
		if strings.Contains(project.Raw, "[["+page.Slug+"]]") {
			continue
		}
		closed := page.Field("closed")
		if closed == "" {
			closed = date.Format("2006-01-02")
		}
		bullet := fmt.Sprintf("- [[%s]] — %s. Closed %s.", page.Slug, page.Title, closed)
		updated := appendToSection(project.Raw, taskArchiveSection, bullet)
		if err := s.FS.WritePage(projectSlug, updated); err != nil {
			warnings = append(warnings, fmt.Sprintf("archive %s: %v", slug, err))
			continue
		}
		project.Raw = updated
		pageBySlug[projectSlug] = project
		archived++
	}
	return archived, warnings, nil
}
