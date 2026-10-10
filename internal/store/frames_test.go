package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"market-mood/internal/market"
	"math"
	"strings"
	"testing"
)

func frameStock() market.Stock {
	return market.Stock{Code: "600000", Name: "旧名称", Eligible: true, Price: market.Ptr(10.), Open: market.Ptr(9.), Change: market.Ptr(1.), Amount: market.Ptr(0.), Sealed: market.Ptr(false), BreakCount: market.Ptr(uint32(0)), LimitStats: &market.LimitStats{Days: 3, Count: 2}, SourceTime: market.Ptr("2026-09-07T10:00:00+08:00"), Bids: []market.BookLevel{{Price: 9.99, Shares: 100}}, FallbackReasons: []string{"主源过期", "备用成功"}}
}
func TestRuntimeIndependentPublicationsAndRestore(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	stock := frameStock()
	frame := market.MarketFrame{Day: "2026-09-07", CapturedAt: "2026-09-07T10:01:00+08:00", SourceTime: stock.SourceTime, Stocks: []market.Stock{stock}, Complete: true, FallbackReasons: []string{"第一来源失败"}}
	if err := s.SaveMarket(ctx, frame); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCalendar(ctx, []string{"2026-09-04", "2026-09-07"}); err != nil {
		t.Fatal(err)
	}
	limits := market.LimitFrame{Day: frame.Day, CapturedAt: frame.CapturedAt, Sealed: []market.Stock{stock}}
	if err := s.SaveLimits(ctx, limits); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSourceError(ctx, "market", "下一轮失败"); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(frame)
	b, _ := json.Marshal(loaded.Market)
	if string(a) != string(b) {
		t.Fatalf("frame changed\n%s\n%s", a, b)
	}
	if loaded.MarketError == nil || loaded.Limits == nil || len(loaded.Calendar) != 2 {
		t.Fatal("independent state lost")
	}
	if err := s.SaveCalendar(ctx, []string{"2026-09-07", "2026-09-08"}); err != nil {
		t.Fatal(err)
	}
	loaded, _ = s.LoadRuntime(ctx)
	if len(loaded.Calendar) != 3 {
		t.Fatal("calendar history pruned")
	}
	var originalID int64
	if err := s.db.QueryRowContext(ctx, "SELECT market_frame FROM runtime_state").Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	frame.Stocks[0].Name = "新名称"
	if err := s.SaveMarket(ctx, frame); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	original, err := loadMarket(ctx, tx, originalID)
	tx.Rollback()
	if err != nil || original.Stocks[0].Name != "旧名称" {
		t.Fatal("old quote changed", original, err)
	}
	loaded, err = s.LoadRuntime(ctx)
	if err != nil || loaded.MarketError != nil || loaded.Market.Stocks[0].Name != "新名称" {
		t.Fatal(loaded.MarketError, err)
	}
}
func TestFrameWriteRollsBackAndCorruptionHasRecordContext(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	stock := frameStock()
	frame := market.MarketFrame{Day: "2026-09-07", CapturedAt: "2026-09-07T10:01:00+08:00", Stocks: []market.Stock{stock}, Complete: true}
	if err := s.SaveMarket(ctx, frame); err != nil {
		t.Fatal(err)
	}
	frame.Stocks[0].Amount = market.Ptr(math.Inf(1))
	if err := s.SaveMarket(ctx, frame); err == nil || !strings.Contains(err.Error(), "amount") {
		t.Fatal("bad float accepted", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM market_frames").Scan(&count); err != nil || count != 1 {
		t.Fatal("partial frame committed", count, err)
	}
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM stock_sets").Scan(&count); err != nil || count != 1 {
		t.Fatal("orphan set committed", count, err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM stock_set_books; DELETE FROM stock_set_fallbacks; DELETE FROM stock_set_rows"); err != nil {
		t.Fatal(err)
	}
	_, err := s.LoadRuntime(ctx)
	if err == nil || !strings.Contains(err.Error(), s.path) || !strings.Contains(err.Error(), "market_frames/1") || !strings.Contains(err.Error(), "数量异常") {
		t.Fatal("corrupt rows became empty market", err)
	}
}
