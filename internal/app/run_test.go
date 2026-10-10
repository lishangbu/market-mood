package app

import (
	"context"
	"market-mood/internal/tasks"
	"testing"
	"time"
)

func TestRunOwnsAllJobsAndJoinsShutdown(t *testing.T) {
	a, _ := newFixture(t)
	jobs := a.jobs()
	if len(jobs) != 10 {
		t.Fatal(len(jobs))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan string, 10)
	cleanup := make(chan struct{})
	for i := range jobs {
		id := jobs[i].id
		jobs[i].work = func(ctx context.Context) error { entered <- id; <-ctx.Done(); <-cleanup; return ctx.Err() }
	}
	done := make(chan error, 1)
	go func() { done <- a.run(ctx, jobs) }()
	seen := map[string]bool{}
	for range 10 {
		select {
		case id := <-entered:
			seen[id] = true
		case <-time.After(2 * time.Second):
			t.Fatal("jobs blocked each other")
		}
	}
	if len(seen) != 10 || !tasks.Healthy(a.Tasks.Snapshot()) {
		t.Fatal(seen, a.Tasks.Snapshot())
	}
	if err := a.Run(ctx); err == nil {
		t.Fatal("duplicate start allowed")
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before workers finished")
	default:
	}
	close(cleanup)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("workers not joined")
	}
	for _, s := range a.Tasks.Snapshot() {
		if s.State != "stopped" {
			t.Fatal(s)
		}
	}
}
func TestWatchMutationWakesCollector(t *testing.T) {
	a, _ := newFixture(t)
	ctx := context.Background()
	a.CollectCalendar(ctx)
	a.CollectMarket(ctx)
	if err := a.AddWatch(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.watchChanged:
	default:
		t.Fatal("add did not notify")
	}
	if err := a.RemoveWatch(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.watchChanged:
	default:
		t.Fatal("delete did not notify")
	}
}
func TestCalendarReadinessWakesDependentsOnce(t *testing.T) {
	a, _ := newFixture(t)
	if err := a.CollectCalendar(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"market", "limits", "top50", "yesterday"} {
		select {
		case <-a.dependencyWake[id]:
		default:
			t.Fatal("not notified", id)
		}
	}
	if err := a.CollectCalendar(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"market", "limits", "top50", "yesterday"} {
		select {
		case <-a.dependencyWake[id]:
			t.Fatal("unchanged calendar resets backoff", id)
		default:
		}
	}
	for _, job := range a.jobs() {
		if channel, ok := a.dependencyWake[job.id]; ok && job.schedule.Wake != channel {
			t.Fatal("scheduler not wired", job.id)
		}
	}
}
