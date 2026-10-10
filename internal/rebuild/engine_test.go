package rebuild

import (
	"context"
	"errors"
	"fmt"
	"market-mood/internal/catalog"
	"market-mood/internal/market"
	"sync"
	"testing"
	"time"
)

type memoryBars struct {
	mu    sync.Mutex
	data  map[string][]market.Bar
	saved int
}

func (c *memoryBars) Bars(ctx context.Context, code, source string) ([]market.Bar, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data[code+source], nil
}
func (c *memoryBars) SaveBars(ctx context.Context, code, source string, rows []market.Bar) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string][]market.Bar{}
	}
	c.data[code+source] = rows
	c.saved++
	return nil
}
func TestHistoryNeedsCorroboratedAbsence(t *testing.T) {
	ctx := context.Background()
	source := func(name string, rows []market.Bar, err error) DailySource {
		return DailySource{Name: name, Fetch: func(context.Context, string, string) ([]market.Bar, error) { return rows, err }}
	}
	validate := func(rows []market.Bar, source string) error {
		_, err := AmountStock(security(), rows, "2026-09-07", source)
		return err
	}
	old := bars()[:2]
	e := &Engine{Cache: &memoryBars{}, Sources: []DailySource{source("通达信历史日线", old, nil), source("新浪历史日线", old, nil)}}
	if _, err := e.history(ctx, security(), "2026-09-07", false, validate); err == nil {
		t.Fatal("two stale sources prove suspension")
	}
	h, err := e.history(ctx, security(), "2026-09-07", true, validate)
	if err != nil || h != nil {
		t.Fatal("verified suspension not used", h, err)
	}
	later := append([]market.Bar{}, old...)
	bar := bars()[2]
	bar.Day = "2026-09-08"
	later = append(later, bar)
	e.Sources[0] = source("通达信历史日线", later, nil)
	h, err = e.history(ctx, security(), "2026-09-07", false, validate)
	if err != nil || h != nil {
		t.Fatal("corroborated missing target", h, err)
	}
	// Any target-day record with a missing amount remains unknown even when the
	// other source lacks the date and the suspension endpoint claims a full day.
	incomplete := bars()
	incomplete[2].Amount = nil
	e.Sources[0] = source("通达信历史日线", incomplete, nil)
	if _, err := e.history(ctx, security(), "2026-09-07", true, validate); err == nil {
		t.Fatal("target record ignored to claim suspension")
	}
}
func TestHistoryCachesOnlyValidatedRows(t *testing.T) {
	cache := &memoryBars{}
	calls := 0
	e := &Engine{Cache: cache, Sources: []DailySource{{Name: "test", Fetch: func(context.Context, string, string) ([]market.Bar, error) { calls++; return bars(), nil }}}}
	validate := func(rows []market.Bar, source string) error {
		_, err := AmountStock(security(), rows, "2026-09-07", source)
		return err
	}
	for range 2 {
		h, err := e.history(context.Background(), security(), "2026-09-07", false, validate)
		if err != nil || h == nil {
			t.Fatal(h, err)
		}
	}
	if calls != 1 || cache.saved != 1 {
		t.Fatal("validated history not reused")
	}
}
func TestWorkerCancellationAndStreaming(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &Engine{Workers: 2}
	slow := security()
	slow.Code = "600001"
	fast := security()
	received := 0
	err := e.walk(ctx, []catalog.Security{slow, fast}, func(ctx context.Context, s catalog.Security) (*market.Stock, string, error) {
		if s.Code == slow.Code {
			<-ctx.Done()
			return nil, "", ctx.Err()
		}
		return &market.Stock{Code: s.Code}, "test", nil
	}, func(o outcome) {
		received++
		if o.err == nil {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || received != 2 {
		t.Fatal("workers not joined", received, err)
	}
}
func TestFullUniverseRankingAndFailureGate(t *testing.T) {
	securities := []catalog.Security{}
	for _, group := range []struct {
		exchange      string
		prefix, count int
	}{{"sh", 600000, 2101}, {"sz", 0, 2000}, {"bj", 920000, 100}} {
		for i := 0; i < group.count; i++ {
			s, _ := catalog.NewSecurity(fmt.Sprintf("%06d", group.prefix+i), "测试", group.exchange, "test")
			s.ListingDate = market.Ptr("1999-11-10")
			securities = append(securities, s)
		}
	}
	cache := &memoryBars{}
	fail := false
	e := &Engine{Cache: cache, Workers: 6, Now: time.Now, Suspended: func(context.Context, string) (map[string]bool, error) { return nil, nil }, Sources: []DailySource{{Name: "test", Fetch: func(ctx context.Context, code, day string) ([]market.Bar, error) {
		if fail && code == "600001" {
			return nil, errors.New("unavailable")
		}
		return bars(), nil
	}}}}
	state, err := e.Top50(context.Background(), securities, "2026-09-07", []string{"2026-09-04", "2026-09-07"}, nil)
	if err != nil || !state.Complete || len(state.Stocks) != 50 || state.Checked != 4101 || state.Excluded != 100 {
		t.Fatal("complete universe not published", state.Checked, state.Failed, err)
	}
	fail = true
	cache.mu.Lock()
	delete(cache.data, "600001test")
	cache.mu.Unlock()
	state, err = e.Top50(context.Background(), securities, "2026-09-07", []string{"2026-09-04", "2026-09-07"}, nil)
	if err != nil || state.Complete || len(state.Stocks) != 0 || state.Failed != 1 {
		t.Fatal("partial ranking published", state.Complete, state.Failed, err)
	}
}
