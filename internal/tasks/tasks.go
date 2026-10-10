// Package tasks tracks collector lifetimes separately from retryable round errors.
package tasks

import (
	"context"
	"fmt"
	"market-mood/internal/trading"
	"slices"
	"strings"
	"sync"
	"time"
)

type Status struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	State         string  `json:"state"`
	StartedAt     *string `json:"started_at"`
	LastCheckedAt *string `json:"last_checked_at"`
	LastSuccessAt *string `json:"last_success_at"`
	Error         *string `json:"error"`
}

type Monitor struct {
	mu       sync.RWMutex
	statuses map[string]Status
	now      func() time.Time
}

func New() *Monitor {
	m := &Monitor{statuses: make(map[string]Status), now: trading.Now}
	for id, name := range map[string]string{
		"calendar": "交易日历", "market": "全市场行情", "limits": "封炸板池",
		"snapshots": "行情存储", "yesterday": "昨日封炸池重建", "top50": "昨日成交前50",
		"turnover": "指数分时", "catalog": "股票目录", "backfill": "历史补录", "watchlist": "自选行情",
	} {
		m.statuses[id] = Status{ID: id, Name: name, State: "starting"}
	}
	return m
}

func (m *Monitor) start(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.statuses[id]
	if !ok {
		return fmt.Errorf("未知采集任务 %q", id)
	}
	if s.State != "starting" {
		return fmt.Errorf("任务 %s 已启动", id)
	}
	at := m.now().Format(time.RFC3339)
	s.State, s.StartedAt = "running", &at
	m.statuses[id] = s
	return nil
}

func (m *Monitor) record(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.statuses[id]
	at := m.now().Format(time.RFC3339)
	s.LastCheckedAt = &at
	if err == nil {
		s.LastSuccessAt, s.Error = &at, nil
	} else {
		message := err.Error()
		s.Error = &message
	}
	m.statuses[id] = s
}

func (m *Monitor) finish(id string, failure *string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.statuses[id]
	s.State = "stopped"
	if failure != nil {
		s.State, s.Error = "failed", failure
	}
	m.statuses[id] = s
}

func copyString(p *string) *string {
	if p == nil {
		return nil
	}
	s := *p
	return &s
}

// Snapshot owns its values, so HTTP serialization never holds a collector lock.
func (m *Monitor) Snapshot() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Status, 0, len(m.statuses))
	for _, s := range m.statuses {
		s.StartedAt, s.LastCheckedAt = copyString(s.StartedAt), copyString(s.LastCheckedAt)
		s.LastSuccessAt, s.Error = copyString(s.LastSuccessAt), copyString(s.Error)
		result = append(result, s)
	}
	slices.SortFunc(result, func(a, b Status) int { return strings.Compare(a.ID, b.ID) })
	return result
}

// Healthy tests liveness; Degraded also includes ordinary retryable work errors.
func Healthy(statuses []Status) bool {
	for _, s := range statuses {
		if s.State != "running" {
			return false
		}
	}
	return len(statuses) != 0
}
func Degraded(statuses []Status) bool {
	if !Healthy(statuses) {
		return true
	}
	for _, s := range statuses {
		if s.Error != nil {
			return true
		}
	}
	return false
}

// Run runs a fixed-interval task. RunScheduled additionally handles trading windows
// and coalesced change notifications. Both join each round before scheduling another.
func (m *Monitor) Run(ctx context.Context, id string, interval time.Duration, work func(context.Context) error) error {
	return m.RunScheduled(ctx, id, Schedule{Interval: interval}, work)
}
