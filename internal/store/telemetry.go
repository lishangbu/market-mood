package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sync"
	"time"
)

type Timing struct {
	Count     uint64 `json:"count"`
	TotalUS   uint64 `json:"total_us"`
	MaxUS     uint64 `json:"max_us"`
	SlowCount uint64 `json:"slow_count"`
}

func (t *Timing) observe(elapsed time.Duration) {
	us := uint64(max(0, elapsed.Microseconds()))
	t.Count++
	if math.MaxUint64-t.TotalUS < us {
		t.TotalUS = math.MaxUint64
	} else {
		t.TotalUS += us
	}
	t.MaxUS = max(t.MaxUS, us)
	if elapsed >= 100*time.Millisecond {
		t.SlowCount++
	}
}

type Operation struct {
	Requested        uint64 `json:"requested"`
	Acquired         uint64 `json:"acquired"`
	AcquireErrors    uint64 `json:"acquire_errors"`
	AcquireCancelled uint64 `json:"acquire_cancelled"`
	WaitTimeouts     uint64 `json:"wait_timeouts"`
	CreateTimeouts   uint64 `json:"create_timeouts"`
	RecycleTimeouts  uint64 `json:"recycle_timeouts"`
	Acquisition      Timing `json:"acquisition"`
	Execution        Timing `json:"execution"`
	ExecutionErrors  uint64 `json:"execution_errors"`
	Panics           uint64 `json:"panics"`
	BusyErrors       uint64 `json:"busy_errors"`
	LockedErrors     uint64 `json:"locked_errors"`
}
type PoolStatus struct {
	MaxSize   int `json:"max_size"`
	Size      int `json:"size"`
	Available int `json:"available"`
	Waiting   int `json:"waiting"`
}
type DatabaseMetrics struct {
	UptimeMS        int64                `json:"uptime_ms"`
	SlowThresholdMS int                  `json:"slow_threshold_ms"`
	Pool            PoolStatus           `json:"pool"`
	Operations      map[string]Operation `json:"operations"`
}
type telemetry struct {
	mu         sync.Mutex
	started    time.Time
	acquiring  int
	operations map[string]Operation
}

func (m *telemetry) update(name string, fn func(*Operation)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value := m.operations[name]
	fn(&value)
	m.operations[name] = value
}
func classifySQL(o *Operation, err error) {
	var sqlite interface{ Code() int }
	if errors.As(err, &sqlite) {
		switch sqlite.Code() & 255 {
		case 5:
			o.BusyErrors++
		case 6:
			o.LockedErrors++
		}
	}
}

// acquire returns a connection and a deferred completion guard. Callers defer
// the guard before row/transaction cleanup so execution includes decoding and
// rollback. No SQL text, dates, symbols or parameter values enter metric keys.
func (s *Store) acquire(ctx context.Context, name string) (*sql.Conn, func(*error), error) {
	started := time.Now()
	m := &s.metrics
	m.mu.Lock()
	value := m.operations[name]
	value.Requested++
	m.operations[name] = value
	m.acquiring++
	m.mu.Unlock()
	conn, err := s.db.Conn(ctx)
	elapsed := time.Since(started)
	m.mu.Lock()
	m.acquiring--
	value = m.operations[name]
	value.Acquisition.observe(elapsed)
	if err != nil {
		value.AcquireErrors++
		if errors.Is(err, context.Canceled) {
			value.AcquireCancelled++
		}
		if errors.Is(err, context.DeadlineExceeded) {
			value.WaitTimeouts++
		}
		classifySQL(&value, err)
	} else {
		value.Acquired++
	}
	m.operations[name] = value
	m.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	execution := time.Now()
	return conn, func(result *error) {
		panicked := recover()
		closeErr := conn.Close()
		if closeErr != nil {
			*result = errors.Join(*result, closeErr)
		}
		m.update(name, func(o *Operation) {
			o.Execution.observe(time.Since(execution))
			if *result != nil || panicked != nil {
				o.ExecutionErrors++
			}
			if panicked != nil {
				o.Panics++
			}
			classifySQL(o, *result)
		})
		if panicked != nil {
			panic(panicked)
		}
	}, nil
}

// DatabaseMetrics never acquires a database connection, even if all are busy.
// Waiting counts acquisitions in flight (including connection creation). Go's
// pool has no separate create/recycle timeout stages; those legacy fields stay 0.
func (s *Store) DatabaseMetrics() DatabaseMetrics {
	stats := s.db.Stats()
	m := &s.metrics
	m.mu.Lock()
	defer m.mu.Unlock()
	result := DatabaseMetrics{UptimeMS: time.Since(m.started).Milliseconds(), SlowThresholdMS: 100,
		Pool: PoolStatus{MaxSize: stats.MaxOpenConnections, Size: stats.OpenConnections, Available: stats.Idle, Waiting: m.acquiring}, Operations: make(map[string]Operation, len(m.operations))}
	for name, value := range m.operations {
		result.Operations[name] = value
	}
	return result
}
