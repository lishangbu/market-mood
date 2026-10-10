package store

import (
	"context"
	"fmt"
	"market-mood/internal/market"
	"testing"
)

func TestRebuildStateRoundtripAndCompletenessGuards(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	state := market.Top50State{Version: 1, Day: "2026-09-07", Total: 100, Checked: 100, Complete: true, UpdatedAt: "2026-09-08T08:00:00+08:00", Stocks: []market.Stock{}}
	for i := 0; i < 50; i++ {
		stock := frameStock()
		stock.Code = fmt.Sprintf("600%03d", i)
		stock.Amount = market.Ptr(1e9)
		state.Stocks = append(state.Stocks, stock)
	}
	if err := s.SaveTop50(ctx, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Top50(ctx)
	if err != nil || loaded == nil || !loaded.Complete || len(loaded.Stocks) != 50 {
		t.Fatal(loaded, err)
	}
	bad := state
	bad.Failed = 1
	if err := s.SaveTop50(ctx, bad); err == nil {
		t.Fatal("failed full ranking persisted")
	}
	loaded, err = s.Top50(ctx)
	if err != nil || loaded.Failed != 0 {
		t.Fatal("bad state replaced valid ranking", err)
	}
	yesterday := market.YesterdayState{Version: 2, Day: "2026-09-07", Checked: 100, Total: 100, Complete: true, UpdatedAt: state.UpdatedAt, Limits: &market.LimitFrame{Day: "2026-09-07", CapturedAt: state.UpdatedAt, Sealed: []market.Stock{frameStock()}}}
	if err := s.SaveYesterday(ctx, yesterday); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Yesterday(ctx)
	if err != nil || restored == nil || !restored.Complete || len(restored.Limits.Sealed) != 1 {
		t.Fatal(restored, err)
	}
	yesterday.Limits.Day = "2026-09-04"
	if err := s.SaveYesterday(ctx, yesterday); err == nil {
		t.Fatal("cross-day rebuild persisted")
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE top50_states SET version=99"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Top50(ctx); err == nil {
		t.Fatal("unknown version loaded")
	}
}
