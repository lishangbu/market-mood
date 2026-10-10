package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/turnover"
)

func (s *Store) SaveBars(ctx context.Context, code, source string, bars []market.Bar) error {
	if !provider.ValidCode(code) || source == "" {
		return fmt.Errorf("日线身份缺失")
	}
	if err := market.ValidateBars(bars); err != nil {
		return err
	}
	return s.transaction(ctx, "daily_bars/"+code, func(tx *sql.Tx) error {
		statement, err := tx.PrepareContext(ctx, `INSERT INTO daily_bars(code,day,source,open,close,high,low,volume,amount,prevclose) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(code,day,source) DO UPDATE SET open=excluded.open,close=excluded.close,high=excluded.high,low=excluded.low,volume=excluded.volume,amount=excluded.amount,prevclose=excluded.prevclose`)
		if err != nil {
			return err
		}
		defer statement.Close()
		for _, b := range bars {
			if _, err := statement.ExecContext(ctx, code, b.Day, source, b.Open, b.Close, b.High, b.Low, b.Volume, b.Amount, b.Prevclose); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Store) Bars(ctx context.Context, code, source string) (_ []market.Bar, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Bars")
	if acquireErr != nil {
		return nil, s.wrap("Bars", acquireErr)
	}
	defer finish(&resultErr)
	rows, err := conn.QueryContext(ctx, "SELECT day,open,close,high,low,volume,amount,prevclose FROM daily_bars WHERE code=? AND source=? ORDER BY day", code, source)
	if err != nil {
		return nil, s.wrap("daily_bars/"+code, err)
	}
	defer rows.Close()
	out := []market.Bar{}
	for rows.Next() {
		var b market.Bar
		if err := rows.Scan(&b.Day, &b.Open, &b.Close, &b.High, &b.Low, &b.Volume, &b.Amount, &b.Prevclose); err != nil {
			return nil, s.wrap("daily_bars/"+code, err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, s.wrap("daily_bars/"+code, err)
	}
	if len(out) > 0 {
		if err := market.ValidateBars(out); err != nil {
			return nil, s.wrap("daily_bars/"+code, err)
		}
	}
	return out, nil
}
func (s *Store) SaveTurnover(ctx context.Context, state turnover.State) error {
	if len(state.Days) > 0 {
		if err := turnover.Validate(state.Days); err != nil {
			return err
		}
	}
	return s.transaction(ctx, "turnover", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO turnover_state VALUES(1,?,?,?) ON CONFLICT(singleton) DO UPDATE SET provider=excluded.provider,captured_at=excluded.captured_at,error=excluded.error", state.Provider, state.CapturedAt, state.Error); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM turnover_active_days"); err != nil {
			return err
		}
		// Keep historical days indefinitely. Each fetched day is replaced atomically
		// so consumers never read half a minute curve. Old dates are never pruned.
		statement, err := tx.PrepareContext(ctx, "INSERT INTO turnover_minutes VALUES(?,?,?)")
		if err != nil {
			return err
		}
		defer statement.Close()
		for day, minutes := range state.Days {
			if _, err := tx.ExecContext(ctx, "INSERT INTO turnover_active_days VALUES(?)", day); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM turnover_minutes WHERE day=?", day); err != nil {
				return err
			}
			for minute, amount := range minutes {
				if _, err := statement.ExecContext(ctx, day, minute, amount); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func (s *Store) Turnover(ctx context.Context) (_ *turnover.State, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Turnover")
	if acquireErr != nil {
		return nil, s.wrap("Turnover", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, s.wrap("turnover", err)
	}
	defer tx.Rollback()
	state := turnover.State{Days: turnover.Days{}}
	err = tx.QueryRowContext(ctx, "SELECT provider,captured_at,error FROM turnover_state WHERE singleton=1").Scan(&state.Provider, &state.CapturedAt, &state.Error)
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, s.wrap("turnover/state", err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT day,minute,amount FROM turnover_minutes WHERE day IN (SELECT day FROM turnover_active_days) ORDER BY day,minute")
	if err != nil {
		return nil, s.wrap("turnover/minutes", err)
	}
	for rows.Next() {
		var day, minute string
		var amount float64
		if err := rows.Scan(&day, &minute, &amount); err != nil {
			rows.Close()
			return nil, s.wrap("turnover/minutes", err)
		}
		if state.Days[day] == nil {
			state.Days[day] = map[string]float64{}
		}
		state.Days[day][minute] = amount
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, s.wrap("turnover/minutes", err)
	}
	if len(state.Days) > 0 {
		if err := turnover.Validate(state.Days); err != nil {
			return nil, s.wrap("turnover/minutes", err)
		}
	}
	return &state, s.wrap("turnover", tx.Commit())
}
