package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gluonfield/jazmem/internal/store/sqlite/generated/statedb"
)

func (s *Store) Doctor(ctx context.Context) (DoctorReport, error) {
	pageCount, err := s.stateQ.CountPages(ctx)
	if err != nil {
		return DoctorReport{}, err
	}
	chunkCount, err := s.stateQ.CountChunks(ctx)
	if err != nil {
		return DoctorReport{}, err
	}
	linkCount, err := s.stateQ.CountLinks(ctx)
	if err != nil {
		return DoctorReport{}, err
	}
	typedLinkCount, err := s.stateQ.CountRelationshipLinks(ctx)
	if err != nil {
		return DoctorReport{}, err
	}
	unresolvedCount, err := s.stateQ.CountUnresolvedLinks(ctx)
	if err != nil {
		return DoctorReport{}, err
	}
	return DoctorReport{
		PageCount:       int(pageCount),
		ChunkCount:      int(chunkCount),
		LinkCount:       int(linkCount),
		TypedLinkCount:  int(typedLinkCount),
		UnresolvedCount: int(unresolvedCount),
	}, nil
}

func (s *Store) Optimize(ctx context.Context) error {
	return s.stateQ.OptimizeFTS(ctx)
}

func (s *Store) RecordTask(ctx context.Context, task, status string, runAt time.Time, errText string) error {
	return s.stateQ.RecordTask(ctx, statedb.RecordTaskParams{
		Task:        task,
		LastRunAtMs: millis(runAt),
		LastStatus:  status,
		LastError:   errText,
	})
}

type TaskStateRow struct {
	Task      string
	LastRunAt time.Time
	Status    string
	Error     string
}

func (s *Store) ListTaskStates(ctx context.Context) ([]TaskStateRow, error) {
	rows, err := s.stateQ.ListTaskStates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TaskStateRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, TaskStateRow{
			Task:      row.Task,
			LastRunAt: time.UnixMilli(row.LastRunAtMs).UTC(),
			Status:    row.LastStatus,
			Error:     row.LastError,
		})
	}
	return out, nil
}

func (s *Store) TaskState(ctx context.Context, task string) (time.Time, string, error) {
	row, err := s.stateQ.GetTaskState(ctx, task)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, "", nil
	}
	if err != nil {
		return time.Time{}, "", err
	}
	status := row.LastStatus
	if row.LastError != "" {
		status += ": " + row.LastError
	}
	return time.UnixMilli(row.LastRunAtMs).UTC(), status, nil
}
