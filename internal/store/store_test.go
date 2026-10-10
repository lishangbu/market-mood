package store

import (
	"context"
	"database/sql"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/turnover"
	"path/filepath"
	"strings"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "market.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestPreferencesPersistAndKeepWatchOrder(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	if err := s.Watch(ctx, "600000", "old"); err != nil {
		t.Fatal(err)
	}
	first, _ := s.Watchlist(ctx)
	if err := s.Watch(ctx, "600000", "new"); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Watchlist(ctx)
	if len(again) != 1 || again[0].Name != "new" || again[0].CreatedAt != first[0].CreatedAt {
		t.Fatal("watch add not idempotent")
	}
	config := provider.DefaultConfig()
	if err := s.SaveConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	bad := config.Clone()
	bad.Quotes = nil
	if err := s.SaveConfig(ctx, bad); err == nil {
		t.Fatal("bad config accepted")
	}
	s.Close()
	reopened, err := Open(ctx, s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved, err := reopened.Config(ctx)
	if err != nil || saved == nil || len(saved.Quotes) != 2 || saved.Quotes[0] != provider.Tencent {
		t.Fatal("config restore", saved, err)
	}
	watches, err := reopened.Watchlist(ctx)
	if err != nil || len(watches) != 1 {
		t.Fatal(watches, err)
	}
	if err := reopened.Unwatch(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Unwatch(ctx, "600000"); err != nil {
		t.Fatal(err)
	}
}
func TestBarsRetainUnknownAndRollbackInvalidSeries(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	bar := market.Bar{Day: "2026-09-07", Open: 10, Close: 11, High: 12, Low: 9, Volume: 0}
	if err := s.SaveBars(ctx, "600000", "新浪", []market.Bar{bar}); err != nil {
		t.Fatal(err)
	}
	bad := bar
	bad.Day = "2026-09-08"
	bad.High = 0
	if err := s.SaveBars(ctx, "600000", "新浪", []market.Bar{bar, bad}); err == nil {
		t.Fatal("invalid series accepted")
	}
	rows, err := s.Bars(ctx, "600000", "新浪")
	if err != nil || len(rows) != 1 || rows[0].Amount != nil || rows[0].Volume != 0 {
		t.Fatal(rows, err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE daily_bars SET high=0"); err != nil {
		t.Fatal(err)
	}
	_, err = s.Bars(ctx, "600000", "新浪")
	if err == nil || !strings.Contains(err.Error(), s.path) || !strings.Contains(err.Error(), "600000") || !strings.Contains(err.Error(), "OHLC") {
		t.Fatal("corrupt data lacks context", err)
	}
}
func TestTurnoverWindowIsAtomicAndDoesNotMixSources(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	a := turnover.State{Days: turnover.Days{"2026-09-07": {"09:30": 100, "09:31": 125}}, Provider: "腾讯行情"}
	if err := s.SaveTurnover(ctx, a); err != nil {
		t.Fatal(err)
	}
	broken := turnover.State{Days: turnover.Days{"2026-09-08": {"09:31": 1}}, Provider: "东方财富"}
	if err := s.SaveTurnover(ctx, broken); err == nil {
		t.Fatal("gap persisted")
	}
	read, err := s.Turnover(ctx)
	if err != nil || read.Provider != a.Provider || len(read.Days) != 1 {
		t.Fatal(read, err)
	}
	b := turnover.State{Days: turnover.Days{"2026-09-08": {"09:30": 200}}, Provider: "东方财富"}
	if err := s.SaveTurnover(ctx, b); err != nil {
		t.Fatal(err)
	}
	read, err = s.Turnover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Days) != 1 || read.Days["2026-09-07"] != nil {
		t.Fatal("prior source window silently reused")
	}
	var retained int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM turnover_minutes WHERE day='2026-09-07'").Scan(&retained); err != nil || retained != 2 {
		t.Fatal("old days pruned", retained, err)
	}
}
func TestRejectForeignDatabaseWithoutModification(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "foreign.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA application_id=1296905030; CREATE TABLE original(value TEXT); INSERT INTO original VALUES('retained')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(ctx, path); err == nil || !strings.Contains(err.Error(), "不自动迁移") {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow("SELECT value FROM original").Scan(&value); err != nil || value != "retained" {
		t.Fatal(value, err)
	}
}
