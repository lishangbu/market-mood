package app

import (
	"context"
	"errors"
	"market-mood/internal/market"
	"market-mood/internal/turnover"
	"testing"
)

func TestTurnoverFailureRetainsWindow(t *testing.T) {
	a, _ := newFixture(t)
	ctx := context.Background()
	state := turnover.State{Days: turnover.Days{"2026-09-07": {"09:30": 100}}, Provider: "test"}
	if err := a.collectTurnover(ctx, func(context.Context, []string) (turnover.State, error) { return state, nil }); err != nil {
		t.Fatal(err)
	}
	if err := a.collectTurnover(ctx, func(context.Context, []string) (turnover.State, error) {
		return turnover.State{}, errors.New("offline")
	}); err == nil {
		t.Fatal("error hidden")
	}
	saved, err := a.Store.Turnover(ctx)
	if err != nil || saved == nil || saved.Error == nil || saved.Days["2026-09-07"]["09:30"] != 100 {
		t.Fatal(saved, err)
	}
}
func TestRebuildRequiresCalendarAndDirectory(t *testing.T) {
	a, _ := newFixture(t)
	ctx := context.Background()
	if err := a.CollectTop50(ctx); err == nil {
		t.Fatal("missing calendar accepted")
	}
	if err := a.CollectYesterday(ctx); err == nil {
		t.Fatal("missing calendar accepted")
	}
	if err := a.CollectCalendar(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.CollectTop50(ctx); err == nil {
		t.Fatal("empty universe accepted")
	}
	if err := a.CollectYesterday(ctx); err == nil {
		t.Fatal("empty universe accepted")
	}
	top, err := a.Store.Top50(ctx)
	if err != nil || top == nil || top.Running || top.Error == nil || top.UpdatedAt == "" {
		t.Fatal(top, err)
	}
	yesterday, err := a.Store.Yesterday(ctx)
	if err != nil || yesterday == nil || yesterday.Running || yesterday.Error == nil || yesterday.UpdatedAt == "" {
		t.Fatal(yesterday, err)
	}
	if err := a.CollectMarket(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.CollectLimits(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.PersistObservation(ctx); err != nil {
		t.Fatal("failed rebuild broke snapshot", err)
	}
}

func TestCompletedRebuildCannotHideMissingDirectory(t *testing.T) {
	a, _ := newFixture(t)
	ctx := context.Background()
	if err := a.CollectCalendar(ctx); err != nil {
		t.Fatal(err)
	}
	day, _, err := a.rebuildTarget()
	if err != nil {
		t.Fatal(err)
	}
	a.yesterday = market.YesterdayState{Day: day, Version: 2, Complete: true, Total: 2233, Checked: 2233}
	a.top50 = market.Top50State{Day: day, Version: 1, Complete: true, Total: 5022, Checked: 5022}
	if err := a.CollectYesterday(ctx); err == nil {
		t.Fatal("reused completed pool from a different directory")
	}
	if err := a.CollectTop50(ctx); err == nil {
		t.Fatal("reused completed ranking from a different directory")
	}
}
