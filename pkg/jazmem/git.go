package jazmem

import (
	"context"

	"github.com/wins/jazmem/internal/memgit"
)

func (m *Memory) Checkpoint(ctx context.Context, message string) (CheckpointReport, error) {
	report, err := memgit.Checkpoint(ctx, m.root, message)
	if err != nil {
		return CheckpointReport{}, err
	}
	return CheckpointReport{
		RepoPath:   report.RepoPath,
		Committed:  report.Committed,
		Commit:     report.Commit,
		Message:    report.Message,
		FilesAdded: report.FilesAdded,
	}, nil
}
