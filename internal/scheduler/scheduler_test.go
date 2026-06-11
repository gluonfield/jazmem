package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeRecorder struct {
	mu     sync.Mutex
	states map[string][3]string // status, errText, runAt(RFC3339Nano)
	times  map[string]time.Time
}

func newFakeRecorder() *fakeRecorder {
	return &fakeRecorder{states: map[string][3]string{}, times: map[string]time.Time{}}
}

func (f *fakeRecorder) RecordTask(_ context.Context, task, status string, runAt time.Time, errText string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	combined := status
	if errText != "" {
		combined += ": " + errText
	}
	f.states[task] = [3]string{combined, errText, ""}
	f.times[task] = runAt
	return nil
}

func (f *fakeRecorder) TaskState(_ context.Context, task string) (time.Time, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.times[task], f.states[task][0], nil
}

func TestFailedCalendarTaskRetriesAfterBackoff(t *testing.T) {
	recorder := newFakeRecorder()
	now := time.Date(2026, 6, 10, 3, 46, 0, 0, time.UTC)
	fail := true
	runs := 0
	task := Task{
		Name: "dream",
		// once-daily style gate: never due again the same day
		Due: func(lastRun, current time.Time) bool {
			return lastRun.IsZero() || lastRun.YearDay() != current.YearDay()
		},
		Run: func(context.Context) error {
			runs++
			if fail {
				return errors.New("dial tcp: lookup openrouter.ai: i/o timeout")
			}
			return nil
		},
	}
	s := &Scheduler{Tasks: []Task{task}, Recorder: recorder, Now: func() time.Time { return now }}

	if err := s.runDue(context.Background()); err != nil || runs != 1 {
		t.Fatalf("first run: err=%v runs=%d", err, runs)
	}

	now = now.Add(10 * time.Minute)
	if err := s.runDue(context.Background()); err != nil || runs != 1 {
		t.Fatalf("within backoff should not retry: err=%v runs=%d", err, runs)
	}

	now = now.Add(25 * time.Minute) // 35m after failure
	fail = false
	if err := s.runDue(context.Background()); err != nil || runs != 2 {
		t.Fatalf("after backoff should retry: err=%v runs=%d", err, runs)
	}

	now = now.Add(time.Hour)
	if err := s.runDue(context.Background()); err != nil || runs != 2 {
		t.Fatalf("successful run consumes the day: err=%v runs=%d", err, runs)
	}
}
