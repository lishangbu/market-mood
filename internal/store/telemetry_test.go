package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestMetricsCountsOperationsAndExecutionFailures(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.Watch(ctx, "600000", "浦发银行"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Watchlist(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.transaction(ctx, "test/failure", func(tx *sql.Tx) error { _, err := tx.ExecContext(ctx, "INSERT INTO nonexistent VALUES(1)"); return err }); err == nil {
		t.Fatal("missing table accepted")
	}
	m := s.DatabaseMetrics()
	for _, name := range []string{"Watch", "Watchlist", "write_test"} {
		o := m.Operations[name]
		if o.Requested != 1 || o.Acquired != 1 || o.Acquisition.Count != 1 || o.Execution.Count != 1 {
			t.Fatal(name, o)
		}
	}
	if m.Operations["write_test"].ExecutionErrors != 1 {
		t.Fatal(m)
	}
	if m.Pool.MaxSize != 4 || m.Pool.Waiting != 0 || m.SlowThresholdMS != 100 {
		t.Fatal(m)
	}
	delete(m.Operations, "Watch")
	if s.DatabaseMetrics().Operations["Watch"].Requested != 1 {
		t.Fatal("snapshot aliases counters")
	}
}
func TestMetricsDuringPoolExhaustionAndCancellation(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	held := []*sql.Conn{}
	for range 4 {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
	}
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()
	pending, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Watchlist(pending); done <- err }()
	deadline := time.Now().Add(time.Second)
	for s.DatabaseMetrics().Pool.Waiting != 1 {
		if time.Now().After(deadline) {
			t.Fatal("acquisition not observed")
		}
		time.Sleep(time.Millisecond)
	}
	m := s.DatabaseMetrics()
	if m.Pool.Available != 0 || m.Pool.Size != 4 || m.Operations["Watchlist"].Acquired != 0 {
		t.Fatal(m)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	o := s.DatabaseMetrics().Operations["Watchlist"]
	if o.AcquireCancelled != 1 || o.AcquireErrors != 1 || o.Execution.Count != 0 || o.Acquisition.Count != 1 {
		t.Fatal(o)
	}
	timeout, stop := context.WithTimeout(ctx, 5*time.Millisecond)
	defer stop()
	if _, err := s.Days(timeout); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if s.DatabaseMetrics().Operations["Days"].WaitTimeouts != 1 {
		t.Fatal("deadline not counted")
	}
}
func TestMetricsPanicRollsBackAndReleases(t *testing.T) {
	s := openTest(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_ = s.transaction(context.Background(), "panic", func(tx *sql.Tx) error {
			if _, err := tx.Exec("INSERT INTO watchlist VALUES('600000','test','now')"); err != nil {
				t.Fatal(err)
			}
			panic("failure")
		})
	}()
	o := s.DatabaseMetrics().Operations["write_panic"]
	if o.Panics != 1 || o.ExecutionErrors != 1 || o.Execution.Count != 1 {
		t.Fatal(o)
	}
	rows, err := s.Watchlist(context.Background())
	if err != nil || len(rows) != 0 {
		t.Fatal("panic transaction persisted", rows, err)
	}
	if s.db.Stats().InUse != 0 {
		t.Fatal("connection leaked")
	}
}
func TestMetricsSQLiteBusyClassification(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	lock, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer lock.ExecContext(ctx, "ROLLBACK")
	// Configure the next idle connection to fail promptly on the real write lock.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout=1"); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if err := s.Watch(ctx, "600000", "test"); err == nil {
		t.Fatal("locked write accepted")
	}
	o := s.DatabaseMetrics().Operations["Watch"]
	if o.BusyErrors != 1 || o.ExecutionErrors != 1 {
		t.Fatal(o)
	}
}
