package tasks

import (
	"context"
	"market-mood/internal/trading"
	"testing"
	"time"
)

func TestTradingWindowResetsIdleDeadline(t *testing.T) {
	policy := Schedule{Interval: time.Minute, IdleInterval: 15 * time.Minute, Window: trading.ActiveWindow}
	at := time.Date(2026, 9, 7, 9, 14, 0, 0, trading.Shanghai)
	state := scheduleState{}
	if !state.ready(at, policy.active(at), false) {
		t.Fatal("first run delayed")
	}
	state.completed(at, at, policy, false)
	if state.ready(at.Add(30*time.Second), false, false) {
		t.Fatal("idle polled too frequently")
	}
	if !state.ready(at.Add(time.Minute), true, false) {
		t.Fatal("opening waited for idle deadline")
	}
	start := at.Add(time.Minute)
	state.completed(start, start.Add(20*time.Second), policy, false)
	if !state.due.Equal(start.Add(time.Minute)) {
		t.Fatal("network latency added to interval")
	}
	if state.ready(start.Add(30*time.Second), true, false) {
		t.Fatal("early retry")
	}
	if !state.ready(start.Add(30*time.Second), true, true) {
		t.Fatal("notification ignored")
	}
	state.completed(start, start.Add(2*time.Minute), policy, true)
	if !state.due.Equal(start.Add(2*time.Minute + time.Second)) {
		t.Fatal("slow round spins", state.due)
	}
}
func TestWakeCoalescesWithoutOverlappingWork(t *testing.T) {
	m := New()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wake := make(chan struct{}, 1)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- m.RunScheduled(ctx, "watchlist", Schedule{Interval: time.Hour, Wake: wake}, func(ctx context.Context) error {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("did not start")
	}
	wake <- struct{}{}
	select {
	case <-entered:
		t.Fatal("overlapping rounds")
	default:
	}
	release <- struct{}{}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("change not observed")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not join work")
	}
}
