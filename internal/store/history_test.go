package store

import (
	"context"
	"encoding/json"
	"market-mood/internal/market"
	"math"
	"strings"
	"testing"
	"time"
)

func dashboardFixture() market.Dashboard {
	at, _ := time.Parse(time.RFC3339, "2026-09-07T10:00:30+08:00")
	s := frameStock()
	data := market.RuntimeData{Calendar: []string{"2026-09-04", "2026-09-07"}, Market: &market.MarketFrame{Day: "2026-09-07", CapturedAt: at.Format(time.RFC3339), SourceTime: s.SourceTime, Stocks: []market.Stock{s}, Complete: true}, Limits: &market.LimitFrame{Day: "2026-09-07", CapturedAt: at.Format(time.RFC3339), Sealed: []market.Stock{s}}, PreviousLimits: &market.LimitFrame{Day: "2026-09-04", CapturedAt: "2026-09-04T15:00:00+08:00", Sealed: []market.Stock{s}}}
	d := market.Build(data, at)
	d.Turnover = &market.TurnoverView{Amount: market.Ptr(1e12), PreviousSameAmount: market.Ptr(8e11), VolumeChange: market.Ptr(25.), Forecast: market.Ptr(2e12), Provider: "test", SourceTime: d.ObservedAt}
	d.Top50Rebuild = &market.Top50State{Version: 1, Day: "2026-09-04", Total: 5000, Checked: 2, Failed: 1, UpdatedAt: d.GeneratedAt}
	d.YesterdayRebuild = &market.YesterdayState{Version: 2, Day: "2026-09-04", Total: 5000, Checked: 2, Failed: 1, UpdatedAt: d.GeneratedAt}
	return d
}
func TestFrozenDashboardRoundtrip(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	d := dashboardFixture()
	before, _ := json.Marshal(d)
	if err := s.SaveSnapshot(ctx, d); err != nil {
		t.Fatal(err)
	}
	views, err := s.History(ctx, "2026-09-07")
	if err != nil || len(views) != 1 {
		t.Fatal(len(views), err)
	}
	after, _ := json.Marshal(views[0])
	if string(before) != string(after) {
		t.Fatalf("snapshot changed\n%s\n%s", before, after)
	}
	d.Score.Version = "future-rules"
	d.Score.Factors[0].Name = "new definition"
	stock := frameStock()
	stock.Name = "new name"
	if err := s.SaveMarket(ctx, market.MarketFrame{Day: "2026-09-07", CapturedAt: d.GeneratedAt, Stocks: []market.Stock{stock}, Complete: true}); err != nil {
		t.Fatal(err)
	}
	views, err = s.History(ctx, "2026-09-07")
	if err != nil {
		t.Fatal(err)
	}
	after, _ = json.Marshal(views[0])
	if string(after) != string(before) {
		t.Fatal("current data rewrote historical snapshot")
	}
	close := market.CloseFrame{Market: market.MarketFrame{Day: "2026-09-07", CapturedAt: d.GeneratedAt, Stocks: []market.Stock{stock}, Complete: true}, Limits: market.LimitFrame{Day: "2026-09-07", CapturedAt: d.GeneratedAt}}
	if err := s.SaveClose(ctx, close, d); err != nil {
		t.Fatal(err)
	}
	view, err := s.CloseView(ctx, "2026-09-07")
	if err != nil || view.Score.Version != "future-rules" || view.Score.Factors[0].Name != "new definition" {
		t.Fatal("historical score recomputed", err)
	}
	restored, err := s.CloseFrame(ctx, "2026-09-07")
	if err != nil || restored.Market.Stocks[0].Name != "new name" {
		t.Fatal(restored, err)
	}
}
func TestSnapshotTimeAndCloseAtomicity(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	d := dashboardFixture()
	for _, observed := range []string{"2026-09-04T10:00:00+08:00", "2026-09-07T09:00:00+08:00", "2026-09-07T10:01:00+08:00"} {
		bad := d
		bad.ObservedAt = &observed
		if err := s.SaveSnapshot(ctx, bad); err == nil {
			t.Fatal("bad observed time accepted", observed)
		}
	}
	close := market.CloseFrame{Market: market.MarketFrame{Day: "2026-09-07", CapturedAt: d.GeneratedAt, Complete: true}, Limits: market.LimitFrame{Day: "2026-09-07", CapturedAt: d.GeneratedAt}}
	d.Score.Total = market.Ptr(math.NaN())
	if err := s.SaveClose(ctx, close, d); err == nil {
		t.Fatal("invalid score committed")
	}
	var count int
	s.db.QueryRowContext(ctx, "SELECT count(*) FROM market_frames").Scan(&count)
	if count != 0 {
		t.Fatal("partial close records committed")
	}
	view, err := s.CloseView(ctx, "2026-09-07")
	if err != nil || view != nil {
		t.Fatal(view, err)
	}
}
func TestMissingFactorRejectedWithContext(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if err := s.SaveSnapshot(ctx, dashboardFixture()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM dashboard_factors WHERE position=7"); err != nil {
		t.Fatal(err)
	}
	_, err := s.History(ctx, "2026-09-07")
	if err == nil || !strings.Contains(err.Error(), s.Path()) || !strings.Contains(err.Error(), "dashboards/") || !strings.Contains(err.Error(), "数量不符") {
		t.Fatal("damaged score silently accepted", err)
	}
}
