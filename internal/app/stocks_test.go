package app

import (
	"context"
	"encoding/json"
	"errors"
	"market-mood/internal/market"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

type stockQuotes struct {
	*quotesFixture
	fetch func(context.Context, string) (market.Stock, error)
}

func (q stockQuotes) Stock(ctx context.Context, code string) (market.Stock, error) {
	return q.fetch(ctx, code)
}
func TestWatchReadsNewestAndKeepsUnknown(t *testing.T) {
	a, q := newFixture(t)
	ctx := context.Background()
	a.CollectCalendar(ctx)
	a.CollectMarket(ctx)
	if err := a.AddWatch(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Watch(ctx, "000001", "平安银行"); err != nil {
		t.Fatal(err)
	}
	// Equivalent offset timestamps must be compared as instants, not strings.
	a.data.WatchQuotes = []market.Stock{{Code: "600000", Name: "浦发银行", Price: market.Ptr(12.), SourceTime: market.Ptr(q.at.Add(time.Minute).UTC().Format(time.RFC3339))}}
	watches, err := a.Watchlist(ctx)
	if err != nil || len(watches.Stocks) != 2 || watches.Stocks[1].Stock.Price == nil || *watches.Stocks[1].Stock.Price != 12 || watches.Stocks[0].Stock.Price != nil || !watches.Stocks[0].Stale {
		t.Fatal(watches, err)
	}
	raw, err := json.Marshal(watches.Stocks[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["stale"]) != "true" || string(fields["price"]) != "null" || string(fields["bids"]) != "[]" {
		t.Fatal(string(raw))
	}
	if err := a.AddWatch(ctx, "bad"); !errors.Is(err, ErrInvalidCode) {
		t.Fatal(err)
	}
	if _, err := a.Stock(ctx, "bad"); !errors.Is(err, ErrInvalidCode) {
		t.Fatal(err)
	}
	if err := a.RemoveWatch(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveWatch(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
}
func TestFailedWatchRetainsQuoteAndDeletedDoesNotReturn(t *testing.T) {
	a, q := newFixture(t)
	ctx := context.Background()
	for _, code := range []string{"600000", "000001"} {
		if err := a.Store.Watch(ctx, code, code); err != nil {
			t.Fatal(err)
		}
	}
	old := market.Stock{Code: "600000", Name: "浦发银行", Price: market.Ptr(9.)}
	a.data.WatchQuotes = []market.Stock{old}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	a.quotes = stockQuotes{q, func(ctx context.Context, code string) (market.Stock, error) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return market.Stock{}, ctx.Err()
		}
		if code == "600000" {
			return market.Stock{}, errors.New("offline")
		}
		return market.Stock{Code: code, Name: code, Price: market.Ptr(10.)}, nil
	}}
	done := make(chan error, 1)
	go func() { done <- a.CollectWatchlist(ctx) }()
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("watch requests not concurrent")
		}
	}
	if err := a.RemoveWatch(ctx, "000001"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("partial source failure hidden")
	}
	state, err := a.Store.LoadRuntime(ctx)
	if err != nil || len(state.WatchQuotes) != 1 || state.WatchQuotes[0].Code != "600000" || *state.WatchQuotes[0].Price != 9 || state.WatchError == nil {
		t.Fatal(state, err)
	}
}
func TestWatchConcurrencyAndVerification(t *testing.T) {
	a, q := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	codes := []string{"600000", "600001", "600002", "600003", "600004", "600005", "600006", "600007", "600008"}
	for _, code := range codes {
		if err := a.Store.Watch(ctx, code, code); err != nil {
			t.Fatal(err)
		}
	}
	var active, peak atomic.Int32
	a.quotes = stockQuotes{q, func(ctx context.Context, code string) (market.Stock, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		select {
		case <-time.After(time.Millisecond):
		case <-ctx.Done():
			return market.Stock{}, ctx.Err()
		}
		return market.Stock{Code: code, Name: code}, nil
	}}
	if err := a.CollectWatchlist(ctx); err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 4 || peak.Load() < 2 {
		t.Fatal(peak.Load())
	}
	slices.Reverse(codes)
	watches, err := a.Watchlist(ctx)
	if err != nil || !slices.Equal(watches.Codes, codes) {
		t.Fatal(watches, err)
	}
	a.quotes = stockQuotes{q, func(context.Context, string) (market.Stock, error) {
		return market.Stock{Code: "600000", Name: "wrong symbol"}, nil
	}}
	if err := a.AddWatch(ctx, "000001"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unverified identity saved", err)
	}
}
