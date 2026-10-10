package app

import (
	"context"
	"errors"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/store"
	"market-mood/internal/trading"
	"market-mood/internal/turnover"
	"path/filepath"
	"testing"
	"time"
)

type quotesFixture struct {
	at        time.Time
	marketErr error
	block     chan struct{}
}

func (q *quotesFixture) Calendar(context.Context) ([]string, error) {
	return []string{"2026-09-04", "2026-09-07"}, nil
}
func (q *quotesFixture) Market(ctx context.Context) (market.MarketFrame, error) {
	if q.block != nil {
		select {
		case <-q.block:
		case <-ctx.Done():
			return market.MarketFrame{}, ctx.Err()
		}
	}
	at := q.at.Format(time.RFC3339)
	return market.MarketFrame{Day: "2026-09-07", CapturedAt: at, SourceTime: &at, Complete: true, Stocks: []market.Stock{{Code: "600000", Name: "浦发银行", Price: market.Ptr(10.), Open: market.Ptr(9.), Change: market.Ptr(1.), Amount: market.Ptr(2e9), Eligible: true, SourceTime: &at}}}, q.marketErr
}
func (q *quotesFixture) Limits(_ context.Context, day string) (market.LimitFrame, error) {
	return market.LimitFrame{Day: day, CapturedAt: q.at.Format(time.RFC3339), Sealed: []market.Stock{}, Broken: []market.Stock{}}, nil
}
func (q *quotesFixture) Stock(context.Context, string) (market.Stock, error) {
	return market.Stock{}, errors.New("unused")
}
func newFixture(t *testing.T) (*App, *quotesFixture) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	client, err := provider.New(provider.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), db, client)
	if err != nil {
		t.Fatal(err)
	}
	q := &quotesFixture{at: time.Date(2026, 9, 7, 10, 0, 0, 0, trading.Shanghai)}
	a.quotes = q
	a.now = func() time.Time { return q.at }
	return a, q
}
func TestCollectFreezeAndStartupValidation(t *testing.T) {
	a, q := newFixture(t)
	ctx := context.Background()
	if err := a.CollectMarket(ctx); err == nil {
		t.Fatal("unverified date published")
	}
	if err := a.CollectCalendar(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.CollectMarket(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.CollectLimits(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.PersistObservation(ctx); err != nil {
		t.Fatal(err)
	}
	history, err := a.Store.History(ctx, "2026-09-07")
	if err != nil || len(history) != 1 {
		t.Fatal(len(history), err)
	}
	q.marketErr = errors.New("source offline")
	if err := a.CollectMarket(ctx); err == nil {
		t.Fatal("source failure hidden")
	}
	d, err := a.Dashboard(ctx)
	if err != nil || d.Breadth == nil || d.Sources[0].Error == nil {
		t.Fatal(d, err)
	}
	q.at = q.at.Add(time.Minute)
	if err := a.PersistObservation(ctx); err != nil {
		t.Fatal(err)
	}
	history, err = a.Store.History(ctx, "2026-09-07")
	if err != nil || len(history) != 1 {
		t.Fatal("failed observation recorded", len(history), err)
	}
	restarted, err := New(ctx, a.Store, a.Provider)
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = a.now
	d, err = restarted.Dashboard(ctx)
	if err != nil || d.Sources[0].Error == nil || *d.Sources[0].Error != "正在验证缓存行情" {
		t.Fatal(d, err)
	}
}
func TestCloseUsesObservedTime(t *testing.T) {
	a, q := newFixture(t)
	ctx := context.Background()
	a.CollectCalendar(ctx)
	q.at = time.Date(2026, 9, 7, 14, 59, 0, 0, trading.Shanghai)
	if err := a.CollectMarket(ctx); err != nil {
		t.Fatal(err)
	}
	q.at = q.at.Add(2 * time.Minute)
	if err := a.CollectLimits(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.PersistObservation(ctx); err != nil {
		t.Fatal(err)
	}
	if close, err := a.Store.CloseFrame(ctx, "2026-09-07"); err != nil || close != nil {
		t.Fatal("saved pre-close quote", close, err)
	}
	if err := a.CollectMarket(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.PersistObservation(ctx); err != nil {
		t.Fatal(err)
	}
	if close, err := a.Store.CloseFrame(ctx, "2026-09-07"); err != nil || close == nil {
		t.Fatal(close, err)
	}
}
func TestSlowMarketDoesNotBlockLimitsOrDashboard(t *testing.T) {
	a, q := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.CollectCalendar(ctx); err != nil {
		t.Fatal(err)
	}
	q.block = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- a.CollectMarket(ctx) }()
	if err := a.CollectLimits(ctx); err != nil {
		t.Fatal(err)
	}
	d, err := a.Dashboard(ctx)
	if err != nil || d.Sealing == nil {
		t.Fatal("slow market blocked independent pool", err)
	}
	close(q.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestIndependentTurnoverAndDurableConfig(t *testing.T) {
	a, q := newFixture(t)
	ctx := context.Background()
	a.CollectCalendar(ctx)
	a.turnover = turnover.State{Days: turnover.Days{"2026-09-07": {"09:30": 100}}, Provider: "test"}
	q.at = time.Date(2026, 9, 7, 9, 30, 0, 0, trading.Shanghai)
	d, err := a.Dashboard(ctx)
	if err != nil || d.Breadth != nil || d.Turnover == nil || d.Turnover.Amount == nil || *d.Turnover.Amount != 100 {
		t.Fatal(d, err)
	}
	config := provider.Config{Quotes: []provider.Source{provider.Eastmoney}, StockQuotes: []provider.Source{provider.Tencent}}
	if err := a.SetConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := a.SetConfig(canceled, provider.DefaultConfig()); err == nil {
		t.Fatal("failed persistence accepted")
	}
	if a.Provider.Config().Quotes[0] != provider.Eastmoney {
		t.Fatal("memory changed before persistence")
	}
	saved, err := a.Store.Config(ctx)
	if err != nil || saved.Quotes[0] != provider.Eastmoney {
		t.Fatal(saved, err)
	}
}
