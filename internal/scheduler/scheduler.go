package scheduler

import (
	"context"
	"time"
)

type Task struct {
	Name     string
	Interval time.Duration
	Due      func(lastRun, now time.Time) bool
	Run      func(context.Context) error
}

type Recorder interface {
	RecordTask(ctx context.Context, task, status string, runAt time.Time, errText string) error
	TaskState(ctx context.Context, task string) (time.Time, string, error)
}

type Scheduler struct {
	Tasks    []Task
	Recorder Recorder
	Now      func() time.Time
}

func (s *Scheduler) Run(ctx context.Context) error {
	if len(s.Tasks) == 0 {
		<-ctx.Done()
		return ctx.Err()
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	if err := s.runDue(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.runDue(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Scheduler) runDue(ctx context.Context) error {
	now := s.now()
	for _, task := range s.Tasks {
		if task.Name == "" || task.Run == nil {
			continue
		}
		lastRun, _, err := s.lastRun(ctx, task.Name)
		if err != nil {
			return err
		}
		if !taskDue(task, lastRun, now) {
			continue
		}
		err = task.Run(ctx)
		status := "ok"
		errText := ""
		if err != nil {
			status = "error"
			errText = err.Error()
		}
		if s.Recorder != nil {
			if recordErr := s.Recorder.RecordTask(ctx, task.Name, status, now, errText); recordErr != nil {
				return recordErr
			}
		}
		if err != nil {
			continue
		}
	}
	return nil
}

func taskDue(task Task, lastRun, now time.Time) bool {
	if task.Due != nil {
		return task.Due(lastRun, now)
	}
	if task.Interval <= 0 {
		return lastRun.IsZero()
	}
	return lastRun.IsZero() || now.Sub(lastRun) >= task.Interval
}

func (s *Scheduler) lastRun(ctx context.Context, task string) (time.Time, string, error) {
	if s.Recorder == nil {
		return time.Time{}, "", nil
	}
	return s.Recorder.TaskState(ctx, task)
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
