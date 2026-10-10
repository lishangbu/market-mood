package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"market-mood/internal/tasks"
	"market-mood/internal/trading"
	"sync"
	"time"
)

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (a *App) wakeDependencies(ids ...string) {
	for _, id := range ids {
		signal(a.dependencyWake[id])
	}
}

type job struct {
	id       string
	schedule tasks.Schedule
	work     func(context.Context) error
}

func (a *App) jobs() []job {
	marketSchedule := tasks.Schedule{Interval: time.Minute, IdleInterval: 15 * time.Minute, Window: trading.ActiveWindow}
	watchSchedule := tasks.Schedule{Interval: 10 * time.Second, IdleInterval: 15 * time.Minute, Window: func(at time.Time) bool {
		a.mu.RLock()
		defer a.mu.RUnlock()
		return trading.Session(a.data.Calendar, at) == "交易中"
	}, Wake: a.watchChanged}
	jobs := []job{
		{"calendar", marketSchedule, a.CollectCalendar},
		{"market", marketSchedule, a.CollectMarket},
		{"limits", marketSchedule, a.CollectLimits},
		{"snapshots", tasks.Schedule{Interval: time.Minute, Wake: a.observationChanged}, a.PersistObservation},
		{"turnover", marketSchedule, a.CollectTurnover},
		{"watchlist", watchSchedule, a.CollectWatchlist},
		{"catalog", tasks.Schedule{Interval: 15 * time.Minute}, a.CollectCatalog},
		{"backfill", tasks.Schedule{Interval: time.Hour}, a.CollectBackfill},
		{"top50", tasks.Schedule{Interval: time.Minute}, a.CollectTop50},
		{"yesterday", tasks.Schedule{Interval: time.Minute}, a.CollectYesterday},
	}
	for i := range jobs {
		if wake, ok := a.dependencyWake[jobs[i].id]; ok {
			jobs[i].schedule.Wake = wake
		}
	}
	return jobs
}

// Run starts all ten independent collectors and joins them on cancellation.
// A failed collector remains visible to health checks while others keep running.
// The owner must wait for Run before closing the database and HTTP transport.
// Each App has a single lifetime; restarting requires loading a new App.
func (a *App) Run(ctx context.Context) error { return a.run(ctx, a.jobs()) }
func (a *App) run(ctx context.Context, jobs []job) error {
	if !a.running.CompareAndSwap(false, true) {
		return fmt.Errorf("采集服务已经启动")
	}
	var wg sync.WaitGroup
	failures := make(chan error, len(jobs))
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.Tasks.RunScheduled(ctx, j.id, j.schedule, j.work); err != nil {
				slog.Error("后台采集任务退出", "task", j.id, "error", err)
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	var result []error
	for err := range failures {
		result = append(result, err)
	}
	return errors.Join(result...)
}
