package jazmem

import (
	"context"
	"os"

	"github.com/wins/jazmem/internal/memfs"
)

func (m *Memory) dailyRollup(ctx context.Context) error {
	date := m.timeNow().Local().Format("2006-01-02")
	slug := "daily/" + date
	path, err := m.fs.PathForSlug(slug)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	content := memfs.FrontmatterString(map[string]string{
		"title": "Daily " + date,
		"type":  "daily",
		"date":  date,
	}) + "# Daily " + date + "\n\n## Inbox\n\n## Notes\n\n## Open Loops\n"
	if err := m.fs.WritePage(slug, content); err != nil {
		return err
	}
	_, err = m.Reindex(ctx, ReindexOptions{})
	return err
}
