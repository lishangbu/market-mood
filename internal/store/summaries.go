package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"market-mood/internal/market"
)

func saveBreadth(ctx context.Context, tx *sql.Tx, value *market.Breadth) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	if _, err := json.Marshal(value); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO breadth_records(up,down,flat,yang,yin,valid,excluded,amount,previous_same_amount,volume_change,forecast) VALUES(?,?,?,?,?,?,?,?,?,?,?)", value.Up, value.Down, value.Flat, value.Yang, value.Yin, value.Valid, value.Excluded, value.Amount, value.PreviousSameAmount, value.VolumeChange, value.Forecast)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	return &id, err
}
func loadBreadth(ctx context.Context, tx *sql.Tx, id *int64) (*market.Breadth, error) {
	if id == nil {
		return nil, nil
	}
	var value market.Breadth
	if err := tx.QueryRowContext(ctx, "SELECT up,down,flat,yang,yin,valid,excluded,amount,previous_same_amount,volume_change,forecast FROM breadth_records WHERE id=?", *id).Scan(&value.Up, &value.Down, &value.Flat, &value.Yang, &value.Yin, &value.Valid, &value.Excluded, &value.Amount, &value.PreviousSameAmount, &value.VolumeChange, &value.Forecast); err != nil {
		return nil, err
	}
	if _, err := json.Marshal(value); err != nil {
		return nil, err
	}
	return &value, nil
}
func saveSealing(ctx context.Context, tx *sql.Tx, value *market.Sealing) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	if _, err := json.Marshal(value); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO sealing_records(sealed,broken,rate,height) VALUES(?,?,?,?)", value.Sealed, value.Broken, value.Rate, value.Height)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	return &id, err
}
func loadSealing(ctx context.Context, tx *sql.Tx, id *int64) (*market.Sealing, error) {
	if id == nil {
		return nil, nil
	}
	var value market.Sealing
	if err := tx.QueryRowContext(ctx, "SELECT sealed,broken,rate,height FROM sealing_records WHERE id=?", *id).Scan(&value.Sealed, &value.Broken, &value.Rate, &value.Height); err != nil {
		return nil, err
	}
	if _, err := json.Marshal(value); err != nil {
		return nil, err
	}
	return &value, nil
}
func saveTurnoverView(ctx context.Context, tx *sql.Tx, value *market.TurnoverView) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	if _, err := json.Marshal(value); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO turnover_views(amount,previous_same_amount,volume_change,forecast,source_time,previous_day,provider,captured_at,stale,error) VALUES(?,?,?,?,?,?,?,?,?,?)", value.Amount, value.PreviousSameAmount, value.VolumeChange, value.Forecast, value.SourceTime, value.PreviousDay, value.Provider, value.CapturedAt, value.Stale, value.Error)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	return &id, err
}
func loadTurnoverView(ctx context.Context, tx *sql.Tx, id *int64) (*market.TurnoverView, error) {
	if id == nil {
		return nil, nil
	}
	var value market.TurnoverView
	if err := tx.QueryRowContext(ctx, "SELECT amount,previous_same_amount,volume_change,forecast,source_time,previous_day,provider,captured_at,stale,error FROM turnover_views WHERE id=?", *id).Scan(&value.Amount, &value.PreviousSameAmount, &value.VolumeChange, &value.Forecast, &value.SourceTime, &value.PreviousDay, &value.Provider, &value.CapturedAt, &value.Stale, &value.Error); err != nil {
		return nil, err
	}
	if _, err := json.Marshal(value); err != nil {
		return nil, err
	}
	return &value, nil
}
