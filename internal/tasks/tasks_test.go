package tasks

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func task(m *Monitor, id string) Status {
	for _, s := range m.Snapshot() {
		if s.ID == id {
			return s
		}
	}
	panic(id)
}
func TestFailureThenSuccess(t *testing.T) {
	m := New()
	at := time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC)
	m.now = func() time.Time { return at }
	if err := m.start("market"); err != nil {
		t.Fatal(err)
	}
	m.record("market", nil)
	success := *task(m, "market").LastSuccessAt
	at = at.Add(time.Minute)
	m.record("market", errors.New("upstream unavailable"))
	s := task(m, "market")
	if s.State != "running" || *s.LastSuccessAt != success || s.Error == nil || !Healthy([]Status{s}) || !Degraded([]Status{s}) {
		t.Fatal(s)
	}
	*s.Error = "mutated snapshot"
	if *task(m, "market").Error != "upstream unavailable" {
		t.Fatal("snapshot aliases state")
	}
	m.record("market", nil)
	if Degraded([]Status{task(m, "market")}) {
		t.Fatal("successful retry not reflected")
	}
}
func TestRunAllAndShutdown(t *testing.T) {
	m := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 10)
	var wg sync.WaitGroup
	for _, s := range m.Snapshot() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.Run(ctx, s.ID, time.Hour, func(context.Context) error { ready <- struct{}{}; return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	for range 10 {
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			t.Fatal("collectors did not start")
		}
	}
	if !Healthy(m.Snapshot()) {
		t.Fatal("all workers should be running")
	}
	if err := m.Run(ctx, "market", time.Hour, func(context.Context) error { return nil }); err == nil {
		t.Fatal("duplicate worker started")
	}
	cancel()
	wg.Wait()
	for _, s := range m.Snapshot() {
		if s.State != "stopped" {
			t.Fatal(s)
		}
	}
}
func TestPanicVisibleAndRetry(t *testing.T) {
	m := New()
	if err := m.Run(context.Background(), "limits", time.Hour, func(context.Context) error { panic("broken invariant") }); err == nil {
		t.Fatal("panic hidden")
	}
	if s := task(m, "limits"); s.State != "failed" || s.Error == nil {
		t.Fatal(s)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rounds := 0
	if err := m.Run(ctx, "market", time.Millisecond, func(context.Context) error {
		rounds++
		if rounds == 1 {
			return errors.New("retry")
		}
		if rounds == 3 {
			cancel()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if rounds != 3 || task(m, "market").LastSuccessAt == nil || task(m, "market").Error != nil {
		t.Fatal(rounds, task(m, "market"))
	}
	if task(m, "limits").State != "failed" {
		t.Fatal("shutdown overwrote failed task")
	}
}
