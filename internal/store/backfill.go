package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/backfill"
	"time"
)

func saveBackfillStatus(ctx context.Context, tx *sql.Tx, status backfill.Status) error {
	if status.Completed < 0 || status.Completed > len(status.TargetDays) {
		return fmt.Errorf("补录进度异常")
	}
	if _, err := time.Parse(time.RFC3339, status.UpdatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE backfill_status SET completed=?,running=?,error=?,updated_at=? WHERE singleton=1", status.Completed, status.Running, status.Error, status.UpdatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM backfill_targets"); err != nil {
		return err
	}
	for i, day := range status.TargetDays {
		if err := validDay(day); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO backfill_targets VALUES(?,?)", i, day); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) SaveBackfillStatus(ctx context.Context, status backfill.Status) error {
	return s.transaction(ctx, "backfill_status", func(tx *sql.Tx) error { return saveBackfillStatus(ctx, tx, status) })
}
func (s *Store) BackfillStatus(ctx context.Context) (_ backfill.Status, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "BackfillStatus")
	if acquireErr != nil {
		return backfill.Status{}, s.wrap("BackfillStatus", acquireErr)
	}
	defer finish(&resultErr)
	status := backfill.Status{TargetDays: []string{}}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return status, s.wrap("backfill_status", err)
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "SELECT completed,running,error,updated_at FROM backfill_status WHERE singleton=1").Scan(&status.Completed, &status.Running, &status.Error, &status.UpdatedAt); err != nil {
		return status, s.wrap("backfill_status", err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT position,day FROM backfill_targets ORDER BY position")
	if err != nil {
		return status, s.wrap("backfill_targets", err)
	}
	for rows.Next() {
		var i int
		var day string
		if err := rows.Scan(&i, &day); err != nil {
			rows.Close()
			return status, s.wrap("backfill_targets", err)
		}
		if i != len(status.TargetDays) {
			rows.Close()
			return status, s.wrap("backfill_targets", fmt.Errorf("position不连续"))
		}
		status.TargetDays = append(status.TargetDays, day)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return status, s.wrap("backfill_targets", err)
	}
	return status, s.wrap("backfill_status", tx.Commit())
}
func saveHistorical(ctx context.Context, tx *sql.Tx, h backfill.HistoricalDay) error {
	if err := backfill.ValidateDay(h); err != nil {
		return err
	}
	var limits *int64
	if h.Limits != nil {
		id, err := saveLimits(ctx, tx, *h.Limits)
		if err != nil {
			return err
		}
		limits = &id
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO historical_days VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(day) DO UPDATE SET fields_version=excluded.fields_version,imported_at=excluded.imported_at,source=excluded.source,sh_open=excluded.sh_open,sh_close=excluded.sh_close,sh_high=excluded.sh_high,sh_low=excluded.sh_low,sh_volume=excluded.sh_volume,sz_open=excluded.sz_open,sz_close=excluded.sz_close,sz_high=excluded.sz_high,sz_low=excluded.sz_low,sz_volume=excluded.sz_volume,limits=excluded.limits,limitation=excluded.limitation`, h.Day, h.FieldsVersion, h.ImportedAt, h.Source, h.Shanghai.Open, h.Shanghai.Close, h.Shanghai.High, h.Shanghai.Low, h.Shanghai.Volume, h.Shenzhen.Open, h.Shenzhen.Close, h.Shenzhen.High, h.Shenzhen.Low, h.Shenzhen.Volume, limits, h.Limitation)
	return err
}
func (s *Store) HistoricalDay(ctx context.Context, day string) (_ *backfill.HistoricalDay, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "HistoricalDay")
	if acquireErr != nil {
		return nil, s.wrap("HistoricalDay", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, s.wrap("historical_days/"+day, err)
	}
	defer tx.Rollback()
	var h backfill.HistoricalDay
	var limits *int64
	err = tx.QueryRowContext(ctx, "SELECT day,fields_version,imported_at,source,sh_open,sh_close,sh_high,sh_low,sh_volume,sz_open,sz_close,sz_high,sz_low,sz_volume,limits,limitation FROM historical_days WHERE day=?", day).Scan(&h.Day, &h.FieldsVersion, &h.ImportedAt, &h.Source, &h.Shanghai.Open, &h.Shanghai.Close, &h.Shanghai.High, &h.Shanghai.Low, &h.Shanghai.Volume, &h.Shenzhen.Open, &h.Shenzhen.Close, &h.Shenzhen.High, &h.Shenzhen.Low, &h.Shenzhen.Volume, &limits, &h.Limitation)
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, s.wrap("historical_days/"+day, err)
	}
	h.Shanghai.Day, h.Shenzhen.Day = h.Day, h.Day
	if limits != nil {
		h.Limits, err = loadLimits(ctx, tx, *limits)
		if err != nil {
			return nil, s.wrap("historical_days/"+day, err)
		}
	}
	if err := backfill.ValidateDay(h); err != nil {
		return nil, s.wrap("historical_days/"+day, err)
	}
	return &h, s.wrap("historical_days/"+day, tx.Commit())
}
func historicalCount(ctx context.Context, tx *sql.Tx, days []string) (int, error) {
	count := 0
	for _, day := range days {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM historical_days WHERE day=? AND fields_version=1", day).Scan(&n); err != nil {
			return 0, err
		}
		count += n
	}
	return count, nil
}
func (s *Store) HistoricalCount(ctx context.Context, days []string) (_ int, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "HistoricalCount")
	if acquireErr != nil {
		return 0, s.wrap("HistoricalCount", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, s.wrap("historical_count", err)
	}
	defer tx.Rollback()
	count, err := historicalCount(ctx, tx, days)
	if err != nil {
		return 0, s.wrap("historical_count", err)
	}
	return count, s.wrap("historical_count", tx.Commit())
}
func (s *Store) SaveBackfillDay(ctx context.Context, h backfill.HistoricalDay, status backfill.Status) (backfill.Status, error) {
	err := s.transaction(ctx, "historical_days/"+h.Day, func(tx *sql.Tx) error {
		if err := saveHistorical(ctx, tx, h); err != nil {
			return err
		}
		count, err := historicalCount(ctx, tx, status.TargetDays)
		if err != nil {
			return err
		}
		status.Completed = count
		return saveBackfillStatus(ctx, tx, status)
	})
	return status, err
}
func (s *Store) Days(ctx context.Context) (_ []string, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Days")
	if acquireErr != nil {
		return nil, s.wrap("Days", acquireErr)
	}
	defer finish(&resultErr)
	rows, err := conn.QueryContext(ctx, "SELECT day FROM snapshots UNION SELECT day FROM closes UNION SELECT day FROM historical_days ORDER BY day DESC")
	if err != nil {
		return nil, s.wrap("history/days", err)
	}
	defer rows.Close()
	days := []string{}
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return nil, s.wrap("history/days", err)
		}
		days = append(days, day)
	}
	return days, s.wrap("history/days", rows.Err())
}
