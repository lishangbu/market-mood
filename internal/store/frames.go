package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/market"
	"slices"
	"time"
)

func frameIdentity(day, captured string) error {
	if err := validDay(day); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, captured); err != nil {
		return fmt.Errorf("captured_at: %w", err)
	}
	return nil
}
func saveMarket(ctx context.Context, tx *sql.Tx, frame market.MarketFrame) (int64, error) {
	if err := frameIdentity(frame.Day, frame.CapturedAt); err != nil {
		return 0, err
	}
	stocks, err := saveStocks(ctx, tx, frame.Stocks)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO market_frames(day,captured_at,source_time,provider,complete,stocks) VALUES(?,?,?,?,?,?)", frame.Day, frame.CapturedAt, frame.SourceTime, frame.Provider, frame.Complete, stocks)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, reason := range frame.FallbackReasons {
		if _, err := tx.ExecContext(ctx, "INSERT INTO market_frame_fallbacks VALUES(?,?,?)", id, i, reason); err != nil {
			return 0, err
		}
	}
	return id, nil
}
func loadMarket(ctx context.Context, tx *sql.Tx, id int64) (*market.MarketFrame, error) {
	frame := market.MarketFrame{FallbackReasons: []string{}}
	var set int64
	if err := tx.QueryRowContext(ctx, "SELECT day,captured_at,source_time,provider,complete,stocks FROM market_frames WHERE id=?", id).Scan(&frame.Day, &frame.CapturedAt, &frame.SourceTime, &frame.Provider, &frame.Complete, &set); err != nil {
		return nil, err
	}
	if err := frameIdentity(frame.Day, frame.CapturedAt); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT position,reason FROM market_frame_fallbacks WHERE frame=? ORDER BY position", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i int
		var reason string
		if err := rows.Scan(&i, &reason); err != nil {
			rows.Close()
			return nil, err
		}
		if i != len(frame.FallbackReasons) {
			rows.Close()
			return nil, fmt.Errorf("frame fallback position不连续")
		}
		frame.FallbackReasons = append(frame.FallbackReasons, reason)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	frame.Stocks, err = loadStocks(ctx, tx, set)
	return &frame, err
}
func saveLimits(ctx context.Context, tx *sql.Tx, frame market.LimitFrame) (int64, error) {
	if err := frameIdentity(frame.Day, frame.CapturedAt); err != nil {
		return 0, err
	}
	sealed, err := saveStocks(ctx, tx, frame.Sealed)
	if err != nil {
		return 0, err
	}
	broken, err := saveStocks(ctx, tx, frame.Broken)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO limit_frames(day,captured_at,sealed,broken) VALUES(?,?,?,?)", frame.Day, frame.CapturedAt, sealed, broken)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func loadLimits(ctx context.Context, tx *sql.Tx, id int64) (*market.LimitFrame, error) {
	var frame market.LimitFrame
	var sealed, broken int64
	if err := tx.QueryRowContext(ctx, "SELECT day,captured_at,sealed,broken FROM limit_frames WHERE id=?", id).Scan(&frame.Day, &frame.CapturedAt, &sealed, &broken); err != nil {
		return nil, err
	}
	if err := frameIdentity(frame.Day, frame.CapturedAt); err != nil {
		return nil, err
	}
	var err error
	frame.Sealed, err = loadStocks(ctx, tx, sealed)
	if err != nil {
		return nil, err
	}
	frame.Broken, err = loadStocks(ctx, tx, broken)
	return &frame, err
}
func (s *Store) SaveMarket(ctx context.Context, frame market.MarketFrame) error {
	return s.transaction(ctx, "market/"+frame.Day, func(tx *sql.Tx) error {
		id, err := saveMarket(ctx, tx, frame)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE runtime_state SET market_frame=?,market_error=NULL WHERE singleton=1", id)
		return err
	})
}
func (s *Store) SaveLimits(ctx context.Context, frame market.LimitFrame) error {
	return s.transaction(ctx, "limits/"+frame.Day, func(tx *sql.Tx) error {
		id, err := saveLimits(ctx, tx, frame)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE runtime_state SET limit_frame=?,limit_error=NULL WHERE singleton=1", id)
		return err
	})
}
func (s *Store) SaveWatchQuotes(ctx context.Context, stocks []market.Stock) error {
	return s.transaction(ctx, "watch_quotes", func(tx *sql.Tx) error {
		id, err := saveStocks(ctx, tx, stocks)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE runtime_state SET watch_stocks=?,watch_error=NULL WHERE singleton=1", id)
		return err
	})
}
func (s *Store) SaveCalendar(ctx context.Context, days []string) error {
	if len(days) == 0 {
		return fmt.Errorf("交易日历为空")
	}
	days = slices.Clone(days)
	slices.Sort(days)
	for i, day := range days {
		if err := validDay(day); err != nil {
			return err
		}
		if i > 0 && day == days[i-1] {
			return fmt.Errorf("交易日历重复")
		}
	}
	return s.transaction(ctx, "calendar", func(tx *sql.Tx) error {
		// Calendar providers return a rolling window. Keep already verified older dates
		// so historical review and predecessor lookup do not decay as years advance.
		for _, day := range days {
			if _, err := tx.ExecContext(ctx, "INSERT INTO calendar_days VALUES(?) ON CONFLICT DO NOTHING", day); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE runtime_state SET calendar_error=NULL WHERE singleton=1")
		return err
	})
}
func (s *Store) RecordSourceError(ctx context.Context, source, message string) (resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "RecordSourceError")
	if acquireErr != nil {
		return s.wrap("RecordSourceError", acquireErr)
	}
	defer finish(&resultErr)
	query, ok := map[string]string{"market": "UPDATE runtime_state SET market_error=? WHERE singleton=1", "limits": "UPDATE runtime_state SET limit_error=? WHERE singleton=1", "calendar": "UPDATE runtime_state SET calendar_error=? WHERE singleton=1", "watch": "UPDATE runtime_state SET watch_error=? WHERE singleton=1"}[source]
	if !ok {
		return fmt.Errorf("未知采集任务 %s", source)
	}
	_, err := conn.ExecContext(ctx, query, message)
	return s.wrap("runtime/"+source, err)
}
func (s *Store) LoadRuntime(ctx context.Context) (_ market.RuntimeData, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "LoadRuntime")
	if acquireErr != nil {
		return market.RuntimeData{}, s.wrap("LoadRuntime", acquireErr)
	}
	defer finish(&resultErr)
	data := market.RuntimeData{Calendar: []string{}, WatchQuotes: []market.Stock{}}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return data, s.wrap("runtime", err)
	}
	defer tx.Rollback()
	var marketID, limitsID, watchID *int64
	err = tx.QueryRowContext(ctx, "SELECT market_frame,limit_frame,watch_stocks,market_error,limit_error,calendar_error,watch_error FROM runtime_state WHERE singleton=1").Scan(&marketID, &limitsID, &watchID, &data.MarketError, &data.LimitError, &data.CalendarError, &data.WatchError)
	if err != nil {
		return data, s.wrap("runtime", err)
	}
	if marketID != nil {
		data.Market, err = loadMarket(ctx, tx, *marketID)
		if err != nil {
			return data, s.wrap(fmt.Sprintf("market_frames/%d", *marketID), err)
		}
	}
	if limitsID != nil {
		data.Limits, err = loadLimits(ctx, tx, *limitsID)
		if err != nil {
			return data, s.wrap(fmt.Sprintf("limit_frames/%d", *limitsID), err)
		}
	}
	if watchID != nil {
		data.WatchQuotes, err = loadStocks(ctx, tx, *watchID)
		if err != nil {
			return data, s.wrap(fmt.Sprintf("stock_sets/%d", *watchID), err)
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT day FROM calendar_days ORDER BY day")
	if err != nil {
		return data, s.wrap("calendar", err)
	}
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			rows.Close()
			return data, s.wrap("calendar", err)
		}
		if err := validDay(day); err != nil {
			rows.Close()
			return data, s.wrap("calendar/"+day, err)
		}
		data.Calendar = append(data.Calendar, day)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, s.wrap("calendar", err)
	}
	return data, s.wrap("runtime", tx.Commit())
}
