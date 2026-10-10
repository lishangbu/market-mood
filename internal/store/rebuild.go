package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/market"
)

func validateTop50(state market.Top50State) error {
	if state.Version != 1 {
		return fmt.Errorf("top50 version=%d 不支持", state.Version)
	}
	if err := frameIdentity(state.Day, state.UpdatedAt); err != nil {
		return err
	}
	if state.Checked < 0 || state.Total < state.Checked || state.Failed < 0 || state.Failed > state.Checked || state.Excluded < 0 {
		return fmt.Errorf("top50 进度计数异常")
	}
	if state.Complete {
		if state.Running || state.Failed != 0 || state.Checked != state.Total || len(state.Stocks) != 50 {
			return fmt.Errorf("top50 完整标记与进度/数量不符")
		}
		seen := map[string]bool{}
		for _, s := range state.Stocks {
			if err := validateStock(s); err != nil {
				return err
			}
			if !s.Eligible || s.Amount == nil || *s.Amount <= 0 || seen[s.Code] {
				return fmt.Errorf("top50 成员金额/身份异常")
			}
			seen[s.Code] = true
		}
	} else if len(state.Stocks) > 0 {
		return fmt.Errorf("top50 不完整时不能发布排名")
	}
	return nil
}
func validateYesterday(state market.YesterdayState) error {
	if state.Version != 2 {
		return fmt.Errorf("yesterday version=%d 不支持", state.Version)
	}
	if err := frameIdentity(state.Day, state.UpdatedAt); err != nil {
		return err
	}
	if state.Checked < 0 || state.Total < state.Checked || state.Failed < 0 || state.Failed > state.Checked || state.Excluded < 0 || state.TdxCount < 0 || state.TdxCount > state.Checked {
		return fmt.Errorf("yesterday 进度计数异常")
	}
	if state.Complete {
		if state.Running || state.Failed != 0 || state.Checked != state.Total || state.Limits == nil || state.Limits.Day != state.Day {
			return fmt.Errorf("yesterday 完整标记与进度/日期不符")
		}
	} else if state.Limits != nil {
		return fmt.Errorf("yesterday 不完整时不能发布名单")
	}
	return nil
}
func saveTop50Record(ctx context.Context, tx *sql.Tx, state market.Top50State) (int64, error) {
	if err := validateTop50(state); err != nil {
		return 0, err
	}
	set, err := saveStocks(ctx, tx, state.Stocks)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO top50_states(version,day,checked,total,excluded,failed,complete,running,error,updated_at,stocks) VALUES(?,?,?,?,?,?,?,?,?,?,?)", state.Version, state.Day, state.Checked, state.Total, state.Excluded, state.Failed, state.Complete, state.Running, state.Error, state.UpdatedAt, set)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}
func (s *Store) SaveTop50(ctx context.Context, state market.Top50State) error {
	return s.transaction(ctx, "top50_states/"+state.Day, func(tx *sql.Tx) error {
		id, err := saveTop50Record(ctx, tx, state)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE rebuild_current SET top50_state=? WHERE singleton=1", id)
		return err
	})
}

func saveYesterdayRecord(ctx context.Context, tx *sql.Tx, state market.YesterdayState) (int64, error) {
	if err := validateYesterday(state); err != nil {
		return 0, err
	}
	var limits *int64
	if state.Limits != nil {
		id, err := saveLimits(ctx, tx, *state.Limits)
		if err != nil {
			return 0, err
		}
		limits = &id
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO yesterday_states(version,day,checked,total,excluded,failed,complete,running,error,updated_at,limits,tdx_count) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", state.Version, state.Day, state.Checked, state.Total, state.Excluded, state.Failed, state.Complete, state.Running, state.Error, state.UpdatedAt, limits, state.TdxCount)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}
func (s *Store) SaveYesterday(ctx context.Context, state market.YesterdayState) error {
	return s.transaction(ctx, "yesterday_states/"+state.Day, func(tx *sql.Tx) error {
		id, err := saveYesterdayRecord(ctx, tx, state)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE rebuild_current SET yesterday_state=? WHERE singleton=1", id)
		return err
	})
}

func loadTop50(ctx context.Context, tx *sql.Tx, id int64) (*market.Top50State, error) {
	var state market.Top50State
	var set int64
	if err := tx.QueryRowContext(ctx, "SELECT version,day,checked,total,excluded,failed,complete,running,error,updated_at,stocks FROM top50_states WHERE id=?", id).Scan(&state.Version, &state.Day, &state.Checked, &state.Total, &state.Excluded, &state.Failed, &state.Complete, &state.Running, &state.Error, &state.UpdatedAt, &set); err != nil {
		return nil, err
	}
	var err error
	state.Stocks, err = loadStocks(ctx, tx, set)
	if err != nil {
		return nil, err
	}
	return &state, validateTop50(state)
}
func loadYesterday(ctx context.Context, tx *sql.Tx, id int64) (*market.YesterdayState, error) {
	var state market.YesterdayState
	var limits *int64
	if err := tx.QueryRowContext(ctx, "SELECT version,day,checked,total,excluded,failed,complete,running,error,updated_at,limits,tdx_count FROM yesterday_states WHERE id=?", id).Scan(&state.Version, &state.Day, &state.Checked, &state.Total, &state.Excluded, &state.Failed, &state.Complete, &state.Running, &state.Error, &state.UpdatedAt, &limits, &state.TdxCount); err != nil {
		return nil, err
	}
	if limits != nil {
		var err error
		state.Limits, err = loadLimits(ctx, tx, *limits)
		if err != nil {
			return nil, err
		}
	}
	return &state, validateYesterday(state)
}
func (s *Store) Top50(ctx context.Context) (_ *market.Top50State, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Top50")
	if acquireErr != nil {
		return nil, s.wrap("Top50", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, s.wrap("top50", err)
	}
	defer tx.Rollback()
	var id *int64
	if err := tx.QueryRowContext(ctx, "SELECT top50_state FROM rebuild_current WHERE singleton=1").Scan(&id); err != nil {
		return nil, s.wrap("top50", err)
	}
	if id == nil {
		return nil, nil
	}
	state, err := loadTop50(ctx, tx, *id)
	if err != nil {
		return nil, s.wrap(fmt.Sprintf("top50_states/%d", *id), err)
	}
	return state, s.wrap("top50", tx.Commit())
}
func (s *Store) Yesterday(ctx context.Context) (_ *market.YesterdayState, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Yesterday")
	if acquireErr != nil {
		return nil, s.wrap("Yesterday", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, s.wrap("yesterday", err)
	}
	defer tx.Rollback()
	var id *int64
	if err := tx.QueryRowContext(ctx, "SELECT yesterday_state FROM rebuild_current WHERE singleton=1").Scan(&id); err != nil {
		return nil, s.wrap("yesterday", err)
	}
	if id == nil {
		return nil, nil
	}
	state, err := loadYesterday(ctx, tx, *id)
	if err != nil {
		return nil, s.wrap(fmt.Sprintf("yesterday_states/%d", *id), err)
	}
	return state, s.wrap("yesterday", tx.Commit())
}
