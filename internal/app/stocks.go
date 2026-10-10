package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"market-mood/internal/catalog"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/trading"
	"strings"
	"time"
)

var ErrInvalidCode = errors.New("请输入沪深北A股六位代码")
var ErrUnavailable = errors.New("该股票行情暂不可用")

type Quote struct {
	Stock market.Stock
	Stale bool
}

func (q Quote) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(q.Stock)
	if err != nil {
		return nil, err
	}
	// Stock owns the normalized wire contract, including empty arrays and nulls.
	suffix := `,"stale":false}`
	if q.Stale {
		suffix = `,"stale":true}`
	}
	return append(raw[:len(raw)-1], suffix...), nil
}

type Watches struct {
	Stocks []Quote  `json:"stocks"`
	Error  *string  `json:"error"`
	Codes  []string `json:"codes"`
}
type PoolDescription struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}
type StockDetail struct {
	Stock Quote             `json:"stock"`
	Pools []PoolDescription `json:"pools"`
	Basic *catalog.Security `json:"basic"`
}

func quote(stock market.Stock, data market.RuntimeData, at time.Time) Quote {
	stock = market.WithLimitFields(stock, data.Limits)
	day := ""
	if stock.SourceTime != nil && len(*stock.SourceTime) >= 10 {
		day = (*stock.SourceTime)[:10]
	}
	return Quote{Stock: stock, Stale: trading.Stale(stock.SourceTime, day, data.Calendar, at)}
}
func newest(data market.RuntimeData, code string) *market.Stock {
	var found *market.Stock
	lists := [][]market.Stock{data.WatchQuotes}
	if data.Market != nil {
		lists = append(lists, data.Market.Stocks)
	}
	for _, list := range lists {
		for _, s := range list {
			if s.Code != code {
				continue
			}
			if found == nil || newer(s.SourceTime, found.SourceTime) {
				copy := s
				found = &copy
			}
		}
	}
	return found
}
func newer(a, b *string) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	ta, ea := time.Parse(time.RFC3339, *a)
	tb, eb := time.Parse(time.RFC3339, *b)
	return ea == nil && (eb != nil || !ta.Before(tb))
}
func event(data market.RuntimeData, code string) *market.Stock {
	if data.Limits != nil {
		for _, list := range [][]market.Stock{data.Limits.Sealed, data.Limits.Broken} {
			for _, s := range list {
				if s.Code == code {
					copy := s
					return &copy
				}
			}
		}
	}
	return nil
}
func basicStock(s catalog.Security) market.Stock {
	result := market.Stock{Code: s.Code, Name: s.Name}
	if s.Industry != nil {
		result.Industry = *s.Industry
	}
	return result
}
func (a *App) Watchlist(ctx context.Context) (Watches, error) {
	watches, err := a.Store.Watchlist(ctx)
	if err != nil {
		return Watches{}, err
	}
	data, _, _, _ := a.snapshot()
	result := Watches{Stocks: []Quote{}, Codes: []string{}, Error: data.WatchError}
	at := a.now()
	for _, w := range watches {
		stock := newest(data, w.Code)
		if stock == nil {
			stock = &market.Stock{Code: w.Code, Name: w.Name}
		}
		result.Stocks = append(result.Stocks, quote(*stock, data, at))
		result.Codes = append(result.Codes, w.Code)
	}
	return result, nil
}
func (a *App) AddWatch(ctx context.Context, code string) error {
	if !provider.ValidCode(code) {
		return ErrInvalidCode
	}
	data, _, _, _ := a.snapshot()
	stock := newest(data, code)
	if stock == nil {
		stock = event(data, code)
	}
	if stock == nil {
		basic, err := a.Store.Security(ctx, code)
		if err != nil {
			return err
		}
		if basic != nil {
			s := basicStock(*basic)
			stock = &s
		}
	}
	if stock == nil {
		s, err := a.quotes.Stock(ctx, code)
		if err != nil {
			return fmt.Errorf("无法验证该股票，请待行情恢复后重试: %w", ErrUnavailable)
		}
		stock = &s
	}
	if stock.Code != code || strings.TrimSpace(stock.Name) == "" {
		return ErrUnavailable
	}
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
	if err := a.Store.Watch(ctx, code, stock.Name); err != nil {
		return err
	}
	signal(a.watchChanged)
	return nil
}
func (a *App) RemoveWatch(ctx context.Context, code string) error {
	if !provider.ValidCode(code) {
		return ErrInvalidCode
	}
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
	if err := a.Store.Unwatch(ctx, code); err != nil {
		return err
	}
	signal(a.watchChanged)
	return nil
}
func (a *App) Stock(ctx context.Context, code string) (StockDetail, error) {
	if !provider.ValidCode(code) {
		return StockDetail{}, ErrInvalidCode
	}
	data, yesterday, top50, amount := a.snapshot()
	stock := newest(data, code)
	if stock == nil {
		stock = event(data, code)
	}
	basic, err := a.Store.Security(ctx, code)
	if err != nil {
		return StockDetail{}, err
	}
	if stock == nil && basic != nil {
		s := basicStock(*basic)
		stock = &s
	}
	if stock == nil {
		s, err := a.quotes.Stock(ctx, code)
		if err != nil {
			return StockDetail{}, ErrUnavailable
		}
		stock = &s
	}
	if stock.Code != code || strings.TrimSpace(stock.Name) == "" {
		return StockDetail{}, ErrUnavailable
	}
	at := a.now()
	dashboard, err := a.dashboardFor(ctx, data, yesterday, top50, amount, at)
	if err != nil {
		return StockDetail{}, err
	}
	result := StockDetail{Stock: quote(*stock, data, at), Basic: basic, Pools: []PoolDescription{}}
	for _, p := range dashboard.Pools {
		for _, s := range p.Stocks {
			if s.Code == code {
				result.Pools = append(result.Pools, PoolDescription{p.Title, p.Description})
				break
			}
		}
	}
	return result, nil
}

// CollectWatchlist bounds source concurrency to four and retains failed symbols'
// last successful quotes. Membership is checked again after requests finish.
func (a *App) CollectWatchlist(ctx context.Context) error {
	a.watchCollectMu.Lock()
	defer a.watchCollectMu.Unlock()
	watches, err := a.Store.Watchlist(ctx)
	if err != nil {
		return a.sourceError(ctx, "watch", err)
	}
	type result struct {
		stock market.Stock
		err   error
	}
	results := make(chan result, 4)
	quotes := map[string]market.Stock{}
	var failures []error
	for start := 0; start < len(watches); start += 4 {
		end := min(start+4, len(watches))
		for _, w := range watches[start:end] {
			go func() {
				s, err := a.quotes.Stock(ctx, w.Code)
				if err == nil && (s.Code != w.Code || strings.TrimSpace(s.Name) == "") {
					err = fmt.Errorf("%s 报价身份不符", w.Code)
				}
				results <- result{s, err}
			}()
		}
		for i := start; i < end; i++ {
			r := <-results
			if r.err != nil {
				failures = append(failures, r.err)
			} else {
				quotes[r.stock.Code] = r.stock
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	a.watchMu.Lock()
	defer a.watchMu.Unlock()
	watches, err = a.Store.Watchlist(ctx)
	if err != nil {
		return a.sourceError(ctx, "watch", err)
	}
	data, _, _, _ := a.snapshot()
	old := map[string]market.Stock{}
	for _, s := range data.WatchQuotes {
		old[s.Code] = s
	}
	stocks := []market.Stock{}
	for _, w := range watches {
		if s, ok := quotes[w.Code]; ok {
			stocks = append(stocks, s)
		} else if s, ok := old[w.Code]; ok {
			stocks = append(stocks, s)
		}
	}
	if err := a.Store.SaveWatchQuotes(ctx, stocks); err != nil {
		return a.sourceError(ctx, "watch", err)
	}
	failure := errors.Join(failures...)
	a.mu.Lock()
	a.data.WatchQuotes = stocks
	a.data.WatchError = nil
	if failure != nil {
		a.data.WatchError = market.Ptr(failure.Error())
	}
	a.mu.Unlock()
	if failure != nil {
		return a.sourceError(ctx, "watch", failure)
	}
	return nil
}
