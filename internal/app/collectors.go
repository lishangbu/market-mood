package app

import (
	"context"
	"errors"
	"fmt"
	"market-mood/internal/backfill"
	"market-mood/internal/catalog"
	"market-mood/internal/market"
	"market-mood/internal/rebuild"
	"market-mood/internal/trading"
	"market-mood/internal/turnover"
	"strings"
	"time"
)

func (a *App) CollectCatalog(ctx context.Context) error {
	status, err := a.Store.CatalogStatus(ctx)
	if err != nil {
		return err
	}
	today := a.now().In(trading.Shanghai).Format(time.DateOnly)
	if status.UpdatedAt != nil && strings.HasPrefix(*status.UpdatedAt, today) {
		rows, err := a.Store.AllSecurities(ctx)
		if err != nil {
			return err
		}
		missing := false
		for _, row := range rows {
			if (row.Exchange == "sh" || row.Exchange == "sz") && row.ListingDate == nil {
				missing = true
				break
			}
		}
		if !missing {
			return nil
		}
		checks := catalog.New(a.Provider.HTTP).SupplementListingDates(ctx, rows)
		for exchange, check := range checks {
			status.ExchangeChecks[exchange] = strings.Split(strings.Split(status.ExchangeChecks[exchange], "；新浪公司资料补充上市日期")[0], "；公司资料及公告补充上市日期")[0] + "；" + check
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := a.Store.ReplaceCatalog(ctx, catalog.Result{Securities: rows, FallbackReasons: status.FallbackReasons, ExchangeChecks: status.ExchangeChecks}); err != nil {
			return err
		}
		useDirectory(a.Provider, rows)
		a.wakeDependencies("top50", "yesterday")
		return nil
	}
	result, err := catalog.New(a.Provider.HTTP).Fetch(ctx)
	if err == nil {
		err = a.Store.ReplaceCatalog(ctx, result)
	}
	if err != nil {
		return errors.Join(err, a.Store.SaveCatalogError(ctx, err.Error()))
	}
	useDirectory(a.Provider, result.Securities)
	a.wakeDependencies("top50", "yesterday")
	return nil
}
func (a *App) CollectTurnover(ctx context.Context) error {
	return a.collectTurnover(ctx, turnover.New(a.Provider.HTTP).Fetch)
}
func (a *App) collectTurnover(ctx context.Context, fetch func(context.Context, []string) (turnover.State, error)) error {
	data, _, _, old := a.snapshot()
	state, err := fetch(ctx, data.Calendar)
	if err != nil {
		state = old
		state.Error = market.Ptr(err.Error())
	}
	// A missing first window cannot be saved as verified minutes. Its failure
	// remains visible in memory; a previous valid window persists with its error.
	var saveErr error
	if len(state.Days) > 0 {
		saveErr = a.Store.SaveTurnover(ctx, state)
	}
	if saveErr != nil {
		state = old
		state.Error = market.Ptr(saveErr.Error())
	}
	a.mu.Lock()
	a.turnover = state
	a.mu.Unlock()
	return errors.Join(err, saveErr)
}
func (a *App) CollectBackfill(ctx context.Context) error {
	e := backfill.Engine{Store: a.Store, Source: backfill.NewClient(a.Provider), Now: a.now}
	return e.Run(ctx)
}
func (a *App) rebuildTarget() (string, []string, error) {
	data, _, _, _ := a.snapshot()
	today := a.now().In(trading.Shanghai).Format(time.DateOnly)
	current := ""
	for _, day := range data.Calendar {
		if day <= today {
			current = max(current, day)
		}
	}
	day := trading.Previous(data.Calendar, current)
	if day == "" {
		return "", nil, fmt.Errorf("等待已核验昨日交易日")
	}
	return day, data.Calendar, nil
}
func (a *App) CollectTop50(ctx context.Context) error {
	day, calendar, err := a.rebuildTarget()
	if err != nil {
		return err
	}
	_, _, previous, _ := a.snapshot()
	rows, err := a.Store.AllSecurities(ctx)
	if err != nil {
		return err
	}
	total := 0
	for _, row := range rows {
		eligible, err := rebuild.Top50Eligible(row, day, calendar)
		if eligible || err != nil {
			total++
		}
	}
	if len(rows) > 0 && previous.Day == day && previous.Version == 1 && previous.Complete && previous.Total == total {
		return nil
	}
	engine := rebuild.New(a.Store, a.Provider.HTTP)
	engine.Now = a.now
	state, err := engine.Top50(ctx, rows, day, calendar, func(state market.Top50State) { a.mu.Lock(); a.top50 = state; a.mu.Unlock() })
	if state.Day != "" {
		state.Running = false
		state.UpdatedAt = a.now().Format(time.RFC3339)
		if err != nil {
			state.Error = market.Ptr(err.Error())
		}
		a.mu.Lock()
		a.top50 = state
		a.mu.Unlock()
		// Keep an interrupted run observable without requiring the cancelled context.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if saveErr := a.Store.SaveTop50(cleanup, state); saveErr != nil {
			return errors.Join(err, saveErr)
		}
	}
	if err != nil {
		return err
	}
	if !state.Complete {
		if state.Error != nil {
			return errors.New(*state.Error)
		}
		return fmt.Errorf("昨日成交前50重建未完成")
	}
	return nil
}
func (a *App) CollectYesterday(ctx context.Context) error {
	day, calendar, err := a.rebuildTarget()
	if err != nil {
		return err
	}
	_, previous, _, _ := a.snapshot()
	rows, err := a.Store.AllSecurities(ctx)
	if err != nil {
		return err
	}
	total := 0
	for _, row := range rows {
		if rebuild.LimitEligible(row, day) {
			total++
		}
	}
	if len(rows) > 0 && previous.Day == day && previous.Version == 2 && previous.Complete && previous.Total == total {
		return nil
	}
	engine := rebuild.New(a.Store, a.Provider.HTTP)
	engine.Now = a.now
	state, err := engine.Yesterday(ctx, rows, day, calendar, func(state market.YesterdayState) { a.mu.Lock(); a.yesterday = state; a.mu.Unlock() })
	if state.Day != "" {
		state.Running = false
		state.UpdatedAt = a.now().Format(time.RFC3339)
		if err != nil {
			state.Error = market.Ptr(err.Error())
		}
		a.mu.Lock()
		a.yesterday = state
		a.mu.Unlock()
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if saveErr := a.Store.SaveYesterday(cleanup, state); saveErr != nil {
			return errors.Join(err, saveErr)
		}
	}
	if err != nil {
		return err
	}
	if !state.Complete {
		if state.Error != nil {
			return errors.New(*state.Error)
		}
		return fmt.Errorf("昨日封炸池重建未完成")
	}
	return nil
}
