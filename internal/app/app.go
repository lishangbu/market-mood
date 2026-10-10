// Package app coordinates immutable observations, persistence and collectors.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"market-mood/internal/catalog"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/store"
	"market-mood/internal/tasks"
	"market-mood/internal/trading"
	"market-mood/internal/turnover"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type Quotes interface {
	Calendar(context.Context) ([]string, error)
	Market(context.Context) (market.MarketFrame, error)
	Limits(context.Context, string) (market.LimitFrame, error)
	Stock(context.Context, string) (market.Stock, error)
}

type App struct {
	Store              *store.Store
	Provider           *provider.Client
	Tasks              *tasks.Monitor
	quotes             Quotes
	now                func() time.Time
	mu                 sync.RWMutex
	data               market.RuntimeData
	yesterday          market.YesterdayState
	top50              market.Top50State
	turnover           turnover.State
	configMu           sync.Mutex
	persistMu          sync.Mutex
	watchMu            sync.Mutex
	watchCollectMu     sync.Mutex
	running            atomic.Bool
	observationChanged chan struct{}
	watchChanged       chan struct{}
	dependencyWake     map[string]chan struct{}
}

// New loads verified caches without treating them as freshly collected quotes.
// The caller owns the store and closes it only after all workers have joined.
func New(ctx context.Context, db *store.Store, client *provider.Client) (*App, error) {
	data, err := db.LoadRuntime(ctx)
	if err != nil {
		return nil, err
	}
	config, err := db.Config(ctx)
	if err != nil {
		return nil, err
	}
	if config != nil {
		if err := client.SetConfig(*config); err != nil {
			return nil, err
		}
	}
	securities, err := db.AllSecurities(ctx)
	if err != nil {
		return nil, err
	}
	if len(securities) > 0 {
		if _, err := catalog.ValidateDirectory(securities); err != nil {
			slog.Warn("缓存目录未通过校验", "error", err)
		} else {
			useDirectory(client, securities)
		}
	}
	a := &App{Store: db, Provider: client, Tasks: tasks.New(), quotes: client, now: trading.Now, data: data}
	a.observationChanged = make(chan struct{}, 1)
	a.watchChanged = make(chan struct{}, 1)
	a.dependencyWake = map[string]chan struct{}{}
	for _, id := range []string{"market", "limits", "top50", "yesterday"} {
		a.dependencyWake[id] = make(chan struct{}, 1)
	}
	a.data.MarketError = market.Ptr("正在验证缓存行情")
	a.data.LimitError = market.Ptr("正在验证缓存行情")
	if state, err := db.Yesterday(ctx); err != nil {
		return nil, err
	} else if state != nil {
		a.yesterday = *state
	}
	if state, err := db.Top50(ctx); err != nil {
		return nil, err
	} else if state != nil {
		a.top50 = *state
	}
	a.yesterday.Running, a.top50.Running = false, false
	if state, err := db.Turnover(ctx); err != nil {
		return nil, err
	} else if state != nil {
		a.turnover = *state
	}
	a.turnover.Error = market.Ptr("正在更新两市分时数据")
	return a, nil
}
func useDirectory(client *provider.Client, rows []catalog.Security) {
	directory := make(map[string]provider.Identity, len(rows))
	for _, s := range rows {
		industry := ""
		if s.Industry != nil {
			industry = *s.Industry
		}
		directory[s.Code] = provider.Identity{Name: s.Name, Industry: industry}
	}
	client.SetDirectory(directory)
}
func (a *App) SetConfig(ctx context.Context, c provider.Config) error {
	c = c.Clone()
	if err := c.Validate(); err != nil {
		return err
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if err := a.Store.SaveConfig(ctx, c); err != nil {
		return err
	}
	return a.Provider.SetConfig(c)
}

// snapshot copies headers under a short lock. Published slices and maps are
// immutable; collectors replace whole observations and never modify older ones.
func (a *App) snapshot() (market.RuntimeData, market.YesterdayState, market.Top50State, turnover.State) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.data, a.yesterday, a.top50, a.turnover
}
func (a *App) dashboardFor(ctx context.Context, data market.RuntimeData, yesterday market.YesterdayState, top50 market.Top50State, amount turnover.State, at time.Time) (market.Dashboard, error) {
	day := ""
	if data.Market != nil {
		day = data.Market.Day
	}
	if data.Limits != nil {
		day = max(day, data.Limits.Day)
	}
	previous := trading.Previous(data.Calendar, day)
	data.Previous = nil
	if previous != "" {
		var err error
		data.Previous, err = a.Store.CloseFrame(ctx, previous)
		if err != nil {
			return market.Dashboard{}, err
		}
	}
	data.PreviousLimits = nil
	if yesterday.Complete && yesterday.Version == 2 {
		data.PreviousLimits = yesterday.Limits
	}
	data.PreviousTop50 = &top50
	d := market.Build(data, at)
	if yesterday.Day != "" {
		d.YesterdayRebuild = &yesterday
	}
	if top50.Day != "" {
		d.Top50Rebuild = &top50
	}
	view := amount.View(data.Calendar, at)
	d.Turnover = &view
	if d.Breadth != nil {
		d.Breadth.Amount, d.Breadth.PreviousSameAmount = view.Amount, view.PreviousSameAmount
		d.Breadth.VolumeChange, d.Breadth.Forecast = view.VolumeChange, view.Forecast
	}
	return d, nil
}
func (a *App) Dashboard(ctx context.Context) (market.Dashboard, error) {
	data, yesterday, top50, amount := a.snapshot()
	return a.dashboardFor(ctx, data, yesterday, top50, amount, a.now())
}

func (a *App) sourceError(ctx context.Context, source string, cause error) error {
	defer signal(a.observationChanged)
	message := cause.Error()
	a.mu.Lock()
	switch source {
	case "calendar":
		a.data.CalendarError = &message
	case "market":
		a.data.MarketError = &message
	case "limits":
		a.data.LimitError = &message
	case "watch":
		a.data.WatchError = &message
	}
	a.mu.Unlock()
	if err := a.Store.RecordSourceError(ctx, source, message); err != nil {
		return fmt.Errorf("%v；保存来源错误: %w", cause, err)
	}
	return cause
}
func (a *App) CollectCalendar(ctx context.Context) error {
	defer signal(a.observationChanged)
	days, err := a.quotes.Calendar(ctx)
	if err == nil {
		err = a.Store.SaveCalendar(ctx, days)
	}
	if err != nil {
		return a.sourceError(ctx, "calendar", err)
	}
	a.mu.Lock()
	days = append(slices.Clone(a.data.Calendar), days...)
	slices.Sort(days)
	days = slices.Compact(days)
	changed := !slices.Equal(a.data.Calendar, days)
	a.data.Calendar = days
	a.data.CalendarError = nil
	a.mu.Unlock()
	if changed {
		a.wakeDependencies("market", "limits", "top50", "yesterday")
	}
	return nil
}
func (a *App) CollectMarket(ctx context.Context) error {
	defer signal(a.observationChanged)
	frame, err := a.quotes.Market(ctx)
	if err == nil {
		a.mu.RLock()
		verified := slices.Contains(a.data.Calendar, frame.Day)
		a.mu.RUnlock()
		if !verified {
			err = fmt.Errorf("行情交易日尚未通过日历核验")
		}
	}
	if err == nil {
		err = a.Store.SaveMarket(ctx, frame)
	}
	if err != nil {
		return a.sourceError(ctx, "market", err)
	}
	a.mu.Lock()
	a.data.Market = &frame
	a.data.MarketError = nil
	a.mu.Unlock()
	return nil
}
func (a *App) CollectLimits(ctx context.Context) error {
	defer signal(a.observationChanged)
	a.mu.RLock()
	day := ""
	today := a.now().In(trading.Shanghai).Format(time.DateOnly)
	for _, d := range a.data.Calendar {
		if d <= today {
			day = max(day, d)
		}
	}
	a.mu.RUnlock()
	if day == "" {
		return a.sourceError(ctx, "limits", fmt.Errorf("等待已核验交易日历"))
	}
	frame, err := a.quotes.Limits(ctx, day)
	if err == nil && frame.Day != day {
		err = fmt.Errorf("封炸池日期与目标交易日不符")
	}
	if err == nil {
		err = a.Store.SaveLimits(ctx, frame)
	}
	if err != nil {
		return a.sourceError(ctx, "limits", err)
	}
	a.mu.Lock()
	a.data.Limits = &frame
	a.data.LimitError = nil
	a.mu.Unlock()
	return nil
}

// PersistObservation freezes one observation across all reads and SQL writes.
func (a *App) PersistObservation(ctx context.Context) error {
	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	data, yesterday, top50, amount := a.snapshot()
	at := a.now().In(trading.Shanghai)
	today, minute := at.Format(time.DateOnly), at.Format("15:04")
	if !slices.Contains(data.Calendar, today) {
		return nil
	}
	d, err := a.dashboardFor(ctx, data, yesterday, top50, amount, at)
	if err != nil {
		return err
	}
	if minute >= "09:30" && minute <= "15:00" && !(minute > "11:30" && minute < "13:00") && d.Day != nil && *d.Day == today && d.ObservedAt != nil {
		ready := true
		for _, s := range d.Sources {
			if s.Stale || s.Error != nil {
				ready = false
			}
		}
		if ready {
			if err := a.Store.SaveSnapshot(ctx, d); err != nil {
				return err
			}
		}
	}
	m, l := data.Market, data.Limits
	if minute < "15:00" || m == nil || l == nil || !m.Complete || m.Day != today || l.Day != today || data.MarketError != nil || data.LimitError != nil || m.SourceTime == nil {
		return nil
	}
	observed, err := time.Parse(time.RFC3339, *m.SourceTime)
	if err != nil {
		return err
	}
	observed = observed.In(trading.Shanghai)
	if observed.After(at) || observed.Format(time.DateOnly) != today || observed.Format("15:04") < "15:00" {
		return nil
	}
	return a.Store.SaveClose(ctx, market.CloseFrame{Market: *m, Limits: *l}, d)
}
