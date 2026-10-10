package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/trading"
	"time"
)

func (s *Store) SaveSnapshot(ctx context.Context, d market.Dashboard) error {
	if d.Day == nil {
		return nil
	}
	day := *d.Day
	if d.ObservedAt == nil {
		return fmt.Errorf("行情观测时间缺失")
	}
	observed, err := time.Parse(time.RFC3339, *d.ObservedAt)
	if err != nil {
		return err
	}
	observed = observed.In(trading.Shanghai)
	captured, err := time.Parse(time.RFC3339, d.GeneratedAt)
	if err != nil {
		return err
	}
	if observed.Format(time.DateOnly) != day {
		return fmt.Errorf("观测时间跨交易日")
	}
	lag := captured.Sub(observed)
	if lag < 0 || lag > 180*time.Second {
		return fmt.Errorf("行情时间过期或超前，不写入盘中记录")
	}
	minute := observed.Format("15:04")
	if !(minute >= "09:30" && minute <= "11:30" || minute >= "13:00" && minute <= "15:00") {
		return fmt.Errorf("不在盘中观测时段")
	}
	return s.transaction(ctx, "snapshots/"+day+"/"+minute, func(tx *sql.Tx) error {
		id, err := saveDashboard(ctx, tx, d)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO snapshots VALUES(?,?,?) ON CONFLICT(day,minute) DO UPDATE SET dashboard=excluded.dashboard", day, minute, id)
		return err
	})
}
func (s *Store) SaveClose(ctx context.Context, close market.CloseFrame, d market.Dashboard) error {
	if d.Day == nil || *d.Day != close.Market.Day || close.Market.Day != close.Limits.Day || !close.Market.Complete {
		return fmt.Errorf("收盘数据日期不一致或市场不完整")
	}
	return s.transaction(ctx, "closes/"+close.Market.Day, func(tx *sql.Tx) error {
		m, err := saveMarket(ctx, tx, close.Market)
		if err != nil {
			return err
		}
		l, err := saveLimits(ctx, tx, close.Limits)
		if err != nil {
			return err
		}
		view, err := saveDashboard(ctx, tx, d)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO closes VALUES(?,?,?,?) ON CONFLICT(day) DO UPDATE SET market=excluded.market,limits=excluded.limits,dashboard=excluded.dashboard", close.Market.Day, m, l, view)
		return err
	})
}
func (s *Store) CloseFrame(ctx context.Context, day string) (_ *market.CloseFrame, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "CloseFrame")
	if acquireErr != nil {
		return nil, s.wrap("CloseFrame", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, s.wrap("closes/"+day, err)
	}
	defer tx.Rollback()
	var m, l int64
	err = tx.QueryRowContext(ctx, "SELECT market,limits FROM closes WHERE day=?", day).Scan(&m, &l)
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, s.wrap("closes/"+day, err)
	}
	frame, err := loadMarket(ctx, tx, m)
	if err != nil {
		return nil, s.wrap("closes/"+day, err)
	}
	limits, err := loadLimits(ctx, tx, l)
	if err != nil {
		return nil, s.wrap("closes/"+day, err)
	}
	if frame.Day != day || limits.Day != day || !frame.Complete {
		return nil, s.wrap("closes/"+day, fmt.Errorf("日期或完整性不符"))
	}
	return &market.CloseFrame{Market: *frame, Limits: *limits}, s.wrap("closes/"+day, tx.Commit())
}
func (s *Store) CloseView(ctx context.Context, day string) (_ *market.Dashboard, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "CloseView")
	if acquireErr != nil {
		return nil, s.wrap("CloseView", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, s.wrap("closes/"+day, err)
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, "SELECT dashboard FROM closes WHERE day=?", day).Scan(&id)
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, s.wrap("closes/"+day, err)
	}
	d, err := loadDashboard(ctx, tx, id)
	if err != nil {
		return nil, s.wrap(fmt.Sprintf("dashboards/%d", id), err)
	}
	return d, s.wrap("closes/"+day, tx.Commit())
}
func (s *Store) History(ctx context.Context, day string) (_ []market.Dashboard, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "History")
	if acquireErr != nil {
		return nil, s.wrap("History", acquireErr)
	}
	defer finish(&resultErr)
	out := []market.Dashboard{}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, s.wrap("snapshots/"+day, err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT dashboard FROM snapshots WHERE day=? ORDER BY minute", day)
	if err != nil {
		return nil, s.wrap("snapshots/"+day, err)
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, s.wrap("snapshots/"+day, err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, s.wrap("snapshots/"+day, err)
	}
	for _, id := range ids {
		d, err := loadDashboard(ctx, tx, id)
		if err != nil {
			return nil, s.wrap(fmt.Sprintf("dashboards/%d", id), err)
		}
		out = append(out, *d)
	}
	return out, s.wrap("snapshots/"+day, tx.Commit())
}
