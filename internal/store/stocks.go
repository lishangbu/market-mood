package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/market"
	"math"
)

const stockColumns = "code,name,industry,price,change,amount,seal_amount,break_count,float_cap,market_cap,turnover_rate,first_seal_time,last_seal_time,open,previous_close,source_time,provider,limit_up,limit_down,boards,sealed,eligible,stats_days,stats_count"

func validateStock(stock market.Stock) error {
	for field, value := range map[string]*float64{"price": stock.Price, "change": stock.Change, "amount": stock.Amount, "seal_amount": stock.SealAmount, "float_cap": stock.FloatCap, "market_cap": stock.MarketCap, "turnover_rate": stock.TurnoverRate, "open": stock.Open, "previous_close": stock.PreviousClose, "limit_up": stock.LimitUp, "limit_down": stock.LimitDown} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fmt.Errorf("股票 %s 字段 %s 非有限值", stock.Code, field)
		}
	}
	for side, levels := range map[string][]market.BookLevel{"bids": stock.Bids, "asks": stock.Asks} {
		for i, level := range levels {
			if math.IsNaN(level.Price) || math.IsInf(level.Price, 0) || math.IsNaN(level.Shares) || math.IsInf(level.Shares, 0) || level.Price < 0 || level.Shares < 0 {
				return fmt.Errorf("股票 %s 字段 %s[%d] 无效盘口", stock.Code, side, i)
			}
		}
	}
	return nil
}
func saveStocks(ctx context.Context, tx *sql.Tx, stocks []market.Stock) (int64, error) {
	result, err := tx.ExecContext(ctx, "INSERT INTO stock_sets(row_count) VALUES(?)", len(stocks))
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	statement, err := tx.PrepareContext(ctx, "INSERT INTO stock_set_rows(set_id,position,"+stockColumns+") VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
	if err != nil {
		return 0, err
	}
	defer statement.Close()
	for position, stock := range stocks {
		if err := validateStock(stock); err != nil {
			return 0, err
		}
		var days, count *uint32
		if stock.LimitStats != nil {
			days, count = &stock.LimitStats.Days, &stock.LimitStats.Count
		}
		if _, err := statement.ExecContext(ctx, id, position, stock.Code, stock.Name, stock.Industry, stock.Price, stock.Change, stock.Amount, stock.SealAmount, stock.BreakCount, stock.FloatCap, stock.MarketCap, stock.TurnoverRate, stock.FirstSealTime, stock.LastSealTime, stock.Open, stock.PreviousClose, stock.SourceTime, stock.Provider, stock.LimitUp, stock.LimitDown, stock.Boards, stock.Sealed, stock.Eligible, days, count); err != nil {
			return 0, fmt.Errorf("股票 %s: %w", stock.Code, err)
		}
		for side, levels := range map[string][]market.BookLevel{"bid": stock.Bids, "ask": stock.Asks} {
			for i, level := range levels {
				if _, err := tx.ExecContext(ctx, "INSERT INTO stock_set_books VALUES(?,?,?,?,?,?)", id, position, side, i, level.Price, level.Shares); err != nil {
					return 0, err
				}
			}
		}
		for i, reason := range stock.FallbackReasons {
			if _, err := tx.ExecContext(ctx, "INSERT INTO stock_set_fallbacks VALUES(?,?,?,?)", id, position, i, reason); err != nil {
				return 0, err
			}
		}
	}
	return id, nil
}
func loadStocks(ctx context.Context, tx *sql.Tx, id int64) ([]market.Stock, error) {
	var expected int
	if err := tx.QueryRowContext(ctx, "SELECT row_count FROM stock_sets WHERE id=?", id).Scan(&expected); err != nil {
		return nil, fmt.Errorf("stock_sets/%d: %w", id, err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT position,"+stockColumns+" FROM stock_set_rows WHERE set_id=? ORDER BY position", id)
	if err != nil {
		return nil, err
	}
	stocks := []market.Stock{}
	for rows.Next() {
		stock := market.Stock{Bids: []market.BookLevel{}, Asks: []market.BookLevel{}, FallbackReasons: []string{}}
		var days, count *uint32
		var position int
		if err := rows.Scan(&position, &stock.Code, &stock.Name, &stock.Industry, &stock.Price, &stock.Change, &stock.Amount, &stock.SealAmount, &stock.BreakCount, &stock.FloatCap, &stock.MarketCap, &stock.TurnoverRate, &stock.FirstSealTime, &stock.LastSealTime, &stock.Open, &stock.PreviousClose, &stock.SourceTime, &stock.Provider, &stock.LimitUp, &stock.LimitDown, &stock.Boards, &stock.Sealed, &stock.Eligible, &days, &count); err != nil {
			rows.Close()
			return nil, err
		}
		if position != len(stocks) {
			rows.Close()
			return nil, fmt.Errorf("stock_sets/%d 非连续position", id)
		}
		if (days == nil) != (count == nil) {
			rows.Close()
			return nil, fmt.Errorf("股票 %s 涨停统计字段不完整", stock.Code)
		}
		if days != nil {
			stock.LimitStats = &market.LimitStats{Days: *days, Count: *count}
		}
		stocks = append(stocks, stock)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(stocks) != expected {
		return nil, fmt.Errorf("stock_sets/%d 数量异常: %d != %d", id, len(stocks), expected)
	}
	rows, err = tx.QueryContext(ctx, "SELECT stock_position,side,position,price,shares FROM stock_set_books WHERE set_id=? ORDER BY stock_position,side,position", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var stockPosition, position int
		var side string
		var level market.BookLevel
		if err := rows.Scan(&stockPosition, &side, &position, &level.Price, &level.Shares); err != nil {
			rows.Close()
			return nil, err
		}
		if stockPosition < 0 || stockPosition >= len(stocks) {
			rows.Close()
			return nil, fmt.Errorf("盘口证券索引异常")
		}
		target := &stocks[stockPosition].Bids
		if side == "ask" {
			target = &stocks[stockPosition].Asks
		} else if side != "bid" {
			rows.Close()
			return nil, fmt.Errorf("盘口side异常")
		}
		if position != len(*target) {
			rows.Close()
			return nil, fmt.Errorf("盘口position不连续")
		}
		*target = append(*target, level)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT stock_position,position,reason FROM stock_set_fallbacks WHERE set_id=? ORDER BY stock_position,position", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var stockPosition, position int
		var reason string
		if err := rows.Scan(&stockPosition, &position, &reason); err != nil {
			rows.Close()
			return nil, err
		}
		if stockPosition < 0 || stockPosition >= len(stocks) || position != len(stocks[stockPosition].FallbackReasons) {
			rows.Close()
			return nil, fmt.Errorf("来源回退索引不连续")
		}
		stocks[stockPosition].FallbackReasons = append(stocks[stockPosition].FallbackReasons, reason)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, stock := range stocks {
		if err := validateStock(stock); err != nil {
			return nil, err
		}
	}
	return stocks, nil
}
