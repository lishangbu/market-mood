package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"market-mood/internal/trading"
	"time"
)

// Schedule uses the active interval inside Window, otherwise IdleInterval.
// A nil Window means a fixed interval. Wake notifications may coalesce.
type Schedule struct {
	Interval     time.Duration
	IdleInterval time.Duration
	Window       func(time.Time) bool
	Wake         <-chan struct{}
}

func (s Schedule) active(at time.Time) bool { return s.Window == nil || s.Window(at) }
func (s Schedule) delay(active bool, failures uint32) time.Duration {
	base := s.Interval
	if !active {
		base = s.IdleInterval
	}
	if failures == 0 {
		return base
	}
	return trading.Backoff(base, failures)
}

type scheduleState struct {
	due      time.Time
	active   bool
	failures uint32
}

func (s *scheduleState) ready(at time.Time, active bool, wake bool) bool {
	if active && !s.active || wake {
		s.due = at
	}
	s.active = active
	return !at.Before(s.due)
}
func (s *scheduleState) completed(start, end time.Time, policy Schedule, failed bool) {
	if !failed {
		s.failures = 0
	} else if s.failures < 32 {
		s.failures++
	}
	s.due = start.Add(policy.delay(s.active, s.failures))
	// Slow calls must yield before the next round, never loop tightly.
	if !s.due.After(end) {
		s.due = end.Add(time.Second)
	}
}

func (m *Monitor) RunScheduled(ctx context.Context, id string, policy Schedule, work func(context.Context) error) (err error) {
	if policy.Interval <= 0 || work == nil || policy.Window != nil && policy.IdleInterval <= 0 {
		return fmt.Errorf("任务 %s 的调度参数无效", id)
	}
	if err := m.start(id); err != nil {
		return err
	}
	stopped := false
	defer func() {
		var failure *string
		if value := recover(); value != nil {
			message := fmt.Sprintf("任务 %s panic: %v", id, value)
			failure = &message
			err = fmt.Errorf("%s", message)
		} else if !stopped {
			message := "任务 " + id + " 意外退出"
			failure = &message
			err = fmt.Errorf("%s", message)
		}
		m.finish(id, failure)
	}()
	state := scheduleState{}
	wake := policy.Wake
	notified := false
	for {
		if ctx.Err() != nil {
			stopped = true
			return nil
		}
		now := m.now()
		if state.ready(now, policy.active(now), notified) {
			notified = false
			started := m.now()
			roundErr := work(ctx)
			if ctx.Err() != nil {
				stopped = true
				return nil
			}
			m.record(id, roundErr)
			ended := m.now()
			state.completed(started, ended, policy, roundErr != nil)
			slog.Debug("采集周期完成", "task", id, "elapsed_ms", ended.Sub(started).Milliseconds(), "error", roundErr, "next", state.due)
		}
		delay := max(time.Millisecond, state.due.Sub(m.now()))
		// Poll only window changes; fixed tasks can sleep until due or notified.
		if policy.Window != nil {
			delay = min(delay, time.Second)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			stopped = true
			return nil
		case _, open := <-wake:
			timer.Stop()
			if !open {
				wake = nil
			} else {
				notified = true
			}
		case <-timer.C:
		}
	}
}
