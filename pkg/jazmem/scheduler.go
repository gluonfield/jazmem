package jazmem

import (
	"context"
	"time"

	"github.com/gluonfield/jazmem/internal/scheduler"
	sqlitestore "github.com/gluonfield/jazmem/internal/store/sqlite"
)

type taskSpec struct {
	name string
	due  func(lastRun, now time.Time) bool
	next func(lastRun, now time.Time) time.Time
	run  func(context.Context) error
}

func intervalSpec(name string, every time.Duration, run func(context.Context) error) taskSpec {
	return taskSpec{
		name: name,
		due: func(lastRun, now time.Time) bool {
			return lastRun.IsZero() || now.Sub(lastRun) >= every
		},
		next: func(lastRun, now time.Time) time.Time {
			if lastRun.IsZero() {
				return now
			}
			return lastRun.Add(every)
		},
		run: run,
	}
}

func dailySpec(name string, hour int, run func(context.Context) error) taskSpec {
	return taskSpec{
		name: name,
		due:  onceDailyAfter(hour),
		next: func(lastRun, now time.Time) time.Time {
			local := now.Local()
			todayAt := time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, local.Location())
			if sameLocalDay(lastRun, now) {
				return todayAt.AddDate(0, 0, 1)
			}
			if local.Before(todayAt) {
				return todayAt
			}
			return now
		},
		run: run,
	}
}

func weeklySpec(name string, hour int, run func(context.Context) error) taskSpec {
	return taskSpec{
		name: name,
		due:  weeklyAfter(hour),
		next: func(lastRun, now time.Time) time.Time {
			if lastRun.IsZero() {
				return now
			}
			return lastRun.Add(7 * 24 * time.Hour)
		},
		run: run,
	}
}

func (m *Memory) taskSpecs() []taskSpec {
	return []taskSpec{
		intervalSpec("index_changed_pages", time.Minute, func(ctx context.Context) error {
			_, err := m.Reindex(ctx, ReindexOptions{})
			return err
		}),
		intervalSpec("ingest_sources", 20*time.Minute, func(ctx context.Context) error {
			_, err := m.ingester.Run(ctx)
			return err
		}),
		dailySpec("daily_rollup", 0, m.dailyRollup),
		dailySpec("link_hygiene", 2, func(ctx context.Context) error {
			_, err := m.LinkHygiene(ctx)
			return err
		}),
		dailySpec("dream", 3, func(ctx context.Context) error {
			_, err := m.Dream(ctx, DreamOptions{})
			return err
		}),
		weeklySpec("optimize_index", 4, m.store.Optimize),
	}
}

func (m *Memory) StartScheduler(ctx context.Context) error {
	specs := m.taskSpecs()
	tasks := make([]scheduler.Task, 0, len(specs))
	for _, spec := range specs {
		tasks = append(tasks, scheduler.Task{Name: spec.name, Due: spec.due, Run: spec.run})
	}
	return (&scheduler.Scheduler{Tasks: tasks, Recorder: m.store, Now: m.timeNow}).Run(ctx)
}

type TaskStatus struct {
	Name      string    `json:"name"`
	LastRunAt time.Time `json:"last_run_at,omitzero"`
	Status    string    `json:"status,omitempty"`
	Error     string    `json:"error,omitempty"`
	NextDue   time.Time `json:"next_due,omitzero"`
}

// SchedulerStatus reports every scheduled task with its last recorded run and
// an estimate of the next due time. Tasks that never ran report a zero
// LastRunAt and are due immediately; failed tasks report the retry time.
func (m *Memory) SchedulerStatus(ctx context.Context) ([]TaskStatus, error) {
	rows, err := m.store.ListTaskStates(ctx)
	if err != nil {
		return nil, err
	}
	byName := map[string]sqlitestore.TaskStateRow{}
	for _, row := range rows {
		byName[row.Task] = row
	}
	now := m.timeNow()
	specs := m.taskSpecs()
	out := make([]TaskStatus, 0, len(specs))
	for _, spec := range specs {
		row := byName[spec.name]
		next := spec.next(row.LastRunAt, now)
		if scheduler.TaskErrored(row.Status) {
			if retry := row.LastRunAt.Add(scheduler.ErrorRetryBackoff); retry.Before(next) {
				next = retry
			}
		}
		out = append(out, TaskStatus{
			Name:      spec.name,
			LastRunAt: row.LastRunAt,
			Status:    row.Status,
			Error:     row.Error,
			NextDue:   next,
		})
	}
	return out, nil
}

func onceDailyAfter(hour int) func(time.Time, time.Time) bool {
	return func(lastRun, now time.Time) bool {
		local := now.Local()
		if local.Hour() < hour {
			return false
		}
		return !sameLocalDay(lastRun, now)
	}
}

func weeklyAfter(hour int) func(time.Time, time.Time) bool {
	return func(lastRun, now time.Time) bool {
		local := now.Local()
		if local.Hour() < hour {
			return false
		}
		if lastRun.IsZero() {
			return true
		}
		return now.Sub(lastRun) >= 7*24*time.Hour
	}
}

func sameLocalDay(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	al := a.Local()
	bl := b.Local()
	return al.Year() == bl.Year() && al.YearDay() == bl.YearDay()
}
