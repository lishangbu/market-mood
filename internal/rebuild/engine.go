package rebuild

import (
	"context"
	"fmt"
	"log/slog"
	"market-mood/internal/catalog"
	"market-mood/internal/market"
	"market-mood/internal/provider/sina"
	"market-mood/internal/provider/tdx"
	"market-mood/internal/provider/tonghuashun"
	"market-mood/internal/trading"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

type BarCache interface {
	Bars(context.Context, string, string) ([]market.Bar, error)
	SaveBars(context.Context, string, string, []market.Bar) error
}
type DailySource struct {
	Name  string
	Fetch func(context.Context, string, string) ([]market.Bar, error)
}
type Engine struct {
	Cache           BarCache
	Sources         []DailySource
	Suspended       func(context.Context, string) (map[string]bool, error)
	CorporateAction func(context.Context, string, string, string) (bool, error)
	Now             func() time.Time
	Workers         int
}

func New(cache BarCache, httpClient *http.Client) *Engine {
	tx, sn, ths, evidence := tdx.New(), sina.New(httpClient), tonghuashun.New(httpClient), NewEvidence(httpClient)
	return &Engine{Cache: cache, Workers: 6, Now: trading.Now, Suspended: evidence.Suspended, CorporateAction: evidence.CorporateAction, Sources: []DailySource{
		{"通达信历史日线", func(ctx context.Context, code, day string) ([]market.Bar, error) { return tx.Daily(ctx, code) }},
		{"新浪历史日线", func(ctx context.Context, code, day string) ([]market.Bar, error) { return sn.Daily(ctx, code) }},
		{tonghuashun.DailySource, ths.Daily},
	}}
}

type history struct {
	rows   []market.Bar
	source string
}

// history returns nil only when missing target bars have independent evidence.
// Source failures alone never establish suspension or a complete historical pool.
func (e *Engine) history(ctx context.Context, s catalog.Security, day string, suspended bool, validate func([]market.Bar, string) error) (*history, error) {
	for _, source := range e.Sources {
		rows, err := e.Cache.Bars(ctx, s.Code, source.Name)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 && validate(rows, source.Name) == nil {
			return &history{rows, source.Name}, nil
		}
	}
	type attempt struct {
		rows []market.Bar
		err  error
	}
	attempts := make([]attempt, len(e.Sources))
	reasons := []string{}
	for i, source := range e.Sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, err := source.Fetch(ctx, s.Code, day)
		if err == nil {
			err = market.ValidateBars(rows)
		}
		attempts[i] = attempt{rows, err}
		if err == nil {
			err = validate(rows, source.Name)
			if err == nil {
				if err := e.Cache.SaveBars(ctx, s.Code, source.Name, rows); err != nil {
					return nil, err
				}
				return &history{rows, source.Name}, nil
			}
		}
		reasons = append(reasons, source.Name+": "+err.Error())
	}
	targetExists, laterExists, successes, sinaMissing := false, false, 0, false
	for i, a := range attempts {
		if a.err != nil {
			continue
		}
		successes++
		found := false
		for _, row := range a.rows {
			if row.Day == day {
				found = true
				targetExists = true
			}
			if row.Day > day {
				laterExists = true
			}
		}
		if e.Sources[i].Name == "新浪历史日线" && !found {
			sinaMissing = true
		}
	}
	if !targetExists && ((suspended && sinaMissing) || (successes >= 2 && laterExists)) {
		return nil, nil
	}
	return nil, fmt.Errorf("%s", strings.Join(reasons, "；"))
}
func (e *Engine) suspensions(ctx context.Context, day string) map[string]bool {
	codes, err := e.Suspended(ctx, day)
	if err != nil {
		slog.Warn("停牌核验不可用，缺失日线保持失败", "day", day, "error", err)
		return map[string]bool{}
	}
	return codes
}

type outcome struct {
	security catalog.Security
	stock    *market.Stock
	source   string
	err      error
}

// walk keeps at most Workers source requests active. One slow security does not
// block scheduling the next when another worker finishes. All workers are joined.
func (e *Engine) walk(ctx context.Context, securities []catalog.Security, fetch func(context.Context, catalog.Security) (*market.Stock, string, error), receive func(outcome)) error {
	jobs := make(chan catalog.Security)
	results := make(chan outcome)
	var wg sync.WaitGroup
	for range min(max(e.Workers, 1), 6) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range jobs {
				stock, source, err := fetch(ctx, s)
				results <- outcome{s, stock, source, err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, s := range securities {
			select {
			case jobs <- s:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	for result := range results {
		receive(result)
	}
	return ctx.Err()
}
func (e *Engine) Top50(ctx context.Context, securities []catalog.Security, day string, calendar []string, progress func(market.Top50State)) (market.Top50State, error) {
	state := market.Top50State{Version: 1, Day: day, Running: true, Stocks: []market.Stock{}}
	candidates := []catalog.Security{}
	if err := validateUniverse(securities); err != nil {
		return state, err
	}
	if !slices.Contains(calendar, day) {
		return state, fmt.Errorf("目标交易日未核验")
	}
	for _, s := range securities {
		eligible, err := Top50Eligible(s, day, calendar)
		if err != nil {
			state.Failed++
			state.Checked++
			state.Error = market.Ptr(s.Code + ": " + err.Error())
		} else if eligible {
			candidates = append(candidates, s)
		} else {
			state.Excluded++
		}
	}
	state.Total = len(candidates) + state.Failed
	publish := func() {
		state.UpdatedAt = e.Now().Format(time.RFC3339)
		if progress != nil {
			progress(state)
		}
	}
	publish()
	suspended := e.suspensions(ctx, day)
	stocks := []market.Stock{}
	err := e.walk(ctx, candidates, func(ctx context.Context, s catalog.Security) (*market.Stock, string, error) {
		h, err := e.history(ctx, s, day, suspended[s.Code], func(rows []market.Bar, source string) error { _, err := AmountStock(s, rows, day, source); return err })
		if err != nil || h == nil {
			return nil, "", err
		}
		stock, err := AmountStock(s, h.rows, day, h.source)
		return &stock, h.source, err
	}, func(result outcome) {
		state.Checked++
		if result.err != nil {
			state.Failed++
			state.Error = market.Ptr(result.security.Code + ": " + result.err.Error())
			slog.Warn("昨日成交额核验失败", "code", result.security.Code, "error", result.err)
		} else if result.stock != nil && result.stock.Amount != nil && *result.stock.Amount > 0 {
			stocks = append(stocks, *result.stock)
		} else {
			state.Excluded++
		}
		publish()
	})
	if err != nil {
		state.Error = market.Ptr(err.Error())
	}
	state = FinishTop50(state, stocks, e.Now())
	if err != nil {
		state.Complete = false
		state.Stocks = []market.Stock{}
		state.Error = market.Ptr(err.Error())
	}
	publish()
	return state, err
}
func (e *Engine) Yesterday(ctx context.Context, securities []catalog.Security, day string, calendar []string, progress func(market.YesterdayState)) (market.YesterdayState, error) {
	state := market.YesterdayState{Version: 2, Day: day, Running: true}
	if err := validateUniverse(securities); err != nil {
		return state, err
	}
	if !slices.Contains(calendar, day) {
		return state, fmt.Errorf("目标交易日未核验")
	}
	prior := trading.Previous(calendar, day)
	if prior == "" {
		return state, fmt.Errorf("前序交易日缺失")
	}
	candidates := []catalog.Security{}
	for _, s := range securities {
		if LimitEligible(s, day) {
			candidates = append(candidates, s)
		} else {
			state.Excluded++
		}
	}
	state.Total = len(candidates)
	publish := func() {
		state.UpdatedAt = e.Now().Format(time.RFC3339)
		if progress != nil {
			progress(state)
		}
	}
	publish()
	suspended := e.suspensions(ctx, day)
	frame := market.LimitFrame{Day: day, CapturedAt: e.Now().Format(time.RFC3339), Sealed: []market.Stock{}, Broken: []market.Stock{}}
	err := e.walk(ctx, candidates, func(ctx context.Context, s catalog.Security) (*market.Stock, string, error) {
		h, err := e.history(ctx, s, day, suspended[s.Code], func(rows []market.Bar, source string) error {
			_, err := Classify(s, rows, day, prior, source)
			return err
		})
		if err != nil || h == nil {
			return nil, "", err
		}
		stock, err := Classify(s, h.rows, day, prior, h.source)
		if err != nil {
			return nil, h.source, err
		}
		if stock != nil {
			action, err := e.CorporateAction(ctx, s.Code, prior, day)
			if err != nil {
				return nil, h.source, err
			}
			if action {
				return nil, h.source, nil
			}
		}
		return stock, h.source, nil
	}, func(result outcome) {
		state.Checked++
		if result.err != nil {
			state.Failed++
			state.Error = market.Ptr(result.security.Code + ": " + result.err.Error())
			slog.Warn("历史封炸池核验失败", "code", result.security.Code, "error", result.err)
		} else {
			if strings.HasPrefix(result.source, "通达信") {
				state.TdxCount++
			}
			if result.stock != nil {
				if result.stock.Sealed != nil && *result.stock.Sealed {
					frame.Sealed = append(frame.Sealed, *result.stock)
				} else {
					frame.Broken = append(frame.Broken, *result.stock)
				}
			}
		}
		publish()
	})
	state.Running = false
	state.UpdatedAt = e.Now().Format(time.RFC3339)
	state.Complete = err == nil && state.Failed == 0 && state.Checked == state.Total
	if state.Complete {
		slices.SortFunc(frame.Sealed, func(a, b market.Stock) int { return strings.Compare(a.Code, b.Code) })
		slices.SortFunc(frame.Broken, func(a, b market.Stock) int { return strings.Compare(a.Code, b.Code) })
		state.Limits = &frame
		state.Error = nil
	} else {
		detail := ""
		if state.Error != nil {
			detail = *state.Error
		}
		if err != nil {
			detail = err.Error()
		}
		state.Error = market.Ptr(fmt.Sprintf("%d只历史数据待补齐：%s", state.Failed, detail))
	}
	publish()
	return state, err
}

func validateUniverse(securities []catalog.Security) error {
	counts, err := catalog.ValidateDirectory(securities)
	if err != nil {
		return err
	}
	if counts["sh"]+counts["sz"] <= 4000 {
		return fmt.Errorf("等待完整沪深股票目录")
	}
	return nil
}
