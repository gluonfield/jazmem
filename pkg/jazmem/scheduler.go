package jazmem

import (
	"context"
	"time"

	"github.com/gluonfield/jazmem/internal/scheduler"
)

func (m *Memory) StartScheduler(ctx context.Context) error {
	tasks := []scheduler.Task{
		{Name: "index_changed_pages", Interval: time.Minute, Run: func(ctx context.Context) error {
			_, err := m.Reindex(ctx, ReindexOptions{})
			return err
		}},
		{Name: "ingest_sources", Interval: 20 * time.Minute, Run: func(ctx context.Context) error {
			_, err := m.ingester.Run(ctx)
			return err
		}},
		{Name: "daily_rollup", Due: onceDailyAfter(0), Run: m.dailyRollup},
		{Name: "dream", Due: onceDailyAfter(3), Run: func(ctx context.Context) error {
			_, err := m.Dream(ctx, DreamOptions{})
			return err
		}},
		{Name: "link_hygiene", Due: onceDailyAfter(2), Run: func(ctx context.Context) error {
			_, err := m.LinkHygiene(ctx)
			return err
		}},
		{Name: "optimize_index", Due: weeklyAfter(4), Run: m.store.Optimize},
	}
	return (&scheduler.Scheduler{Tasks: tasks, Recorder: m.store, Now: m.timeNow}).Run(ctx)
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
