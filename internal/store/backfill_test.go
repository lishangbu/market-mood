package store

import (
	"context"
	"errors"
	"market-mood/internal/backfill"
	"market-mood/internal/market"
	"market-mood/internal/trading"
	"slices"
	"testing"
	"time"
)

type historySource struct {
	bars      []backfill.DailyBar
	failPools bool
	calls     int
}

func (s *historySource) Index(context.Context, string) ([]backfill.DailyBar, error) {
	return s.bars, nil
}
func (s *historySource) Limits(_ context.Context, day string) (market.LimitFrame, string, error) {
	s.calls++
	if s.failPools {
		return market.LimitFrame{}, "", errors.New("pool offline")
	}
	return market.LimitFrame{Day: day, CapturedAt: day + "T16:00:00+08:00", Sealed: []market.Stock{}, Broken: []market.Stock{}}, "test", nil
}
func historicalBars() []backfill.DailyBar {
	bars := []backfill.DailyBar{}
	for _, day := range []string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04", "2026-09-07", "2026-09-08"} {
		bars = append(bars, backfill.DailyBar{Day: day, Open: 10, Close: 11, High: 12, Low: 9, Volume: 100})
	}
	return bars
}
func TestBackfillPartialThenRepairAndFreeze(t *testing.T) {
	ctx := context.Background()
	db := openTest(t)
	source := &historySource{bars: historicalBars(), failPools: true}
	at := time.Date(2026, 9, 8, 15, 59, 0, 0, trading.Shanghai)
	engine := backfill.Engine{Store: db, Source: source, Now: func() time.Time { return at }}
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := db.BackfillStatus(ctx)
	if err != nil || status.Running || status.Completed != 5 || status.Error != nil {
		t.Fatal(status, err)
	}
	want := []string{"2026-09-07", "2026-09-04", "2026-09-03", "2026-09-02", "2026-09-01"}
	days, err := db.Days(ctx)
	if err != nil || !slices.Equal(days, want) {
		t.Fatal(days, err)
	}
	h, err := db.HistoricalDay(ctx, want[0])
	if err != nil || h.Limits != nil || h.Limitation == nil {
		t.Fatal(h, err)
	}
	source.failPools = false
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}
	h, err = db.HistoricalDay(ctx, want[0])
	if err != nil || h.Limits == nil {
		t.Fatal(h, err)
	}
	source.bars[4].Close = 10.5
	if err := engine.Run(ctx); err != nil {
		t.Fatal(err)
	}
	h, err = db.HistoricalDay(ctx, want[0])
	if err != nil || h.Shanghai.Close != 11 || source.calls != 10 {
		t.Fatal("cached complete history was rewritten", h, source.calls, err)
	}
	// A short upstream response must record failure without destroying prior progress.
	source.bars = source.bars[:2]
	if err := engine.Run(ctx); err == nil {
		t.Fatal("short history accepted")
	}
	status, err = db.BackfillStatus(ctx)
	if err != nil || status.Running || status.Error == nil || status.Completed != 5 || !slices.Equal(status.TargetDays, want) {
		t.Fatal(status, err)
	}
}
func TestBackfillTransactionRollback(t *testing.T) {
	db := openTest(t)
	ctx := context.Background()
	bar := historicalBars()[0]
	h := backfill.HistoricalDay{FieldsVersion: 1, Day: bar.Day, ImportedAt: bar.Day + "T16:00:00+08:00", Source: "test", Shanghai: bar, Shenzhen: bar}
	bad := backfill.Status{TargetDays: []string{bar.Day}, UpdatedAt: "invalid"}
	if _, err := db.SaveBackfillDay(ctx, h, bad); err == nil {
		t.Fatal("invalid status accepted")
	}
	if got, err := db.HistoricalDay(ctx, bar.Day); err != nil || got != nil {
		t.Fatal("partial transaction persisted", got, err)
	}
	good := bad
	good.UpdatedAt = h.ImportedAt
	if _, err := db.SaveBackfillDay(ctx, h, good); err != nil {
		t.Fatal(err)
	}
	h.Shenzhen.Day = "2026-09-02"
	if _, err := db.SaveBackfillDay(ctx, h, good); err == nil {
		t.Fatal("mixed dates accepted")
	}
	if got, err := db.HistoricalDay(ctx, bar.Day); err != nil || got.Shenzhen.Day != bar.Day {
		t.Fatal(got, err)
	}
}
func TestBackfillCancellationPersistsFailure(t *testing.T) {
	db := openTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	source := &cancellingHistory{historySource: historySource{bars: historicalBars()}, cancel: cancel}
	engine := backfill.Engine{Store: db, Source: source, Now: func() time.Time { return time.Date(2026, 9, 8, 15, 0, 0, 0, trading.Shanghai) }}
	if err := engine.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s, err := db.BackfillStatus(context.Background())
	if err != nil || s.Running || s.Error == nil {
		t.Fatal(s, err)
	}
}

type cancellingHistory struct {
	historySource
	cancel context.CancelFunc
}

func (s *cancellingHistory) Limits(ctx context.Context, day string) (market.LimitFrame, string, error) {
	s.cancel()
	return market.LimitFrame{}, "", ctx.Err()
}
