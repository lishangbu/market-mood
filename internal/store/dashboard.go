package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"market-mood/internal/market"
	"time"
)

func validateDashboard(d market.Dashboard) error {
	if _, err := json.Marshal(d); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, d.GeneratedAt); err != nil {
		return fmt.Errorf("generated_at: %w", err)
	}
	if d.Day != nil {
		if err := validDay(*d.Day); err != nil {
			return err
		}
	}
	for _, p := range d.Pools {
		if p.Count != len(p.Stocks) || p.ValidCount < 0 || p.ValidCount > p.Count {
			return fmt.Errorf("pool %s 样本数量不符", p.ID)
		}
	}
	return nil
}
func saveDashboard(ctx context.Context, tx *sql.Tx, d market.Dashboard) (int64, error) {
	if err := validateDashboard(d); err != nil {
		return 0, err
	}
	breadth, err := saveBreadth(ctx, tx, d.Breadth)
	if err != nil {
		return 0, err
	}
	capacity, err := saveSealing(ctx, tx, d.Capacity)
	if err != nil {
		return 0, err
	}
	sealing, err := saveSealing(ctx, tx, d.Sealing)
	if err != nil {
		return 0, err
	}
	turnover, err := saveTurnoverView(ctx, tx, d.Turnover)
	if err != nil {
		return 0, err
	}
	var top50, yesterday *int64
	if d.Top50Rebuild != nil {
		id, err := saveTop50Record(ctx, tx, *d.Top50Rebuild)
		if err != nil {
			return 0, err
		}
		top50 = &id
	}
	if d.YesterdayRebuild != nil {
		id, err := saveYesterdayRecord(ctx, tx, *d.YesterdayRebuild)
		if err != nil {
			return 0, err
		}
		yesterday = &id
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO dashboards(day,generated_at,observed_at,session,previous_day,score_version,score_total,breadth,capacity,sealing,turnover,top50,yesterday,pool_count,factor_count,source_count,note_count) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, d.Day, d.GeneratedAt, d.ObservedAt, d.Session, d.PreviousDay, d.Score.Version, d.Score.Total, breadth, capacity, sealing, turnover, top50, yesterday, len(d.Pools), len(d.Score.Factors), len(d.Sources), len(d.Notes))
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, p := range d.Pools {
		set, err := saveStocks(ctx, tx, p.Stocks)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO dashboard_pools VALUES(?,?,?,?,?,?,?,?,?,?,?)", id, i, p.ID, p.Title, p.Description, p.Ready, p.Count, p.ValidCount, p.Average, p.YangRate, set); err != nil {
			return 0, err
		}
	}
	for i, f := range d.Score.Factors {
		if _, err := tx.ExecContext(ctx, "INSERT INTO dashboard_factors VALUES(?,?,?,?,?,?,?,?,?)", id, i, f.Name, f.Weight, f.Value, f.Score, f.Unit, f.Range[0], f.Range[1]); err != nil {
			return 0, err
		}
	}
	for i, source := range d.Sources {
		if _, err := tx.ExecContext(ctx, "INSERT INTO dashboard_sources VALUES(?,?,?,?,?,?,?,?,?)", id, i, source.Name, source.CapturedAt, source.SourceTime, source.Provider, source.Error, source.Stale, len(source.FallbackReasons)); err != nil {
			return 0, err
		}
		for j, reason := range source.FallbackReasons {
			if _, err := tx.ExecContext(ctx, "INSERT INTO dashboard_source_fallbacks VALUES(?,?,?,?)", id, i, j, reason); err != nil {
				return 0, err
			}
		}
	}
	for i, note := range d.Notes {
		if _, err := tx.ExecContext(ctx, "INSERT INTO dashboard_notes VALUES(?,?,?)", id, i, note); err != nil {
			return 0, err
		}
	}
	return id, nil
}
func loadDashboard(ctx context.Context, tx *sql.Tx, id int64) (*market.Dashboard, error) {
	d := market.Dashboard{Pools: []market.Pool{}, Sources: []market.SourceStatus{}, Notes: []string{}, Score: market.Score{Factors: []market.Factor{}}}
	var breadth, capacity, sealing, turnover, top50, yesterday *int64
	var poolCount, factorCount, sourceCount, noteCount int
	err := tx.QueryRowContext(ctx, `SELECT day,generated_at,observed_at,session,previous_day,score_version,score_total,breadth,capacity,sealing,turnover,top50,yesterday,pool_count,factor_count,source_count,note_count FROM dashboards WHERE id=?`, id).Scan(&d.Day, &d.GeneratedAt, &d.ObservedAt, &d.Session, &d.PreviousDay, &d.Score.Version, &d.Score.Total, &breadth, &capacity, &sealing, &turnover, &top50, &yesterday, &poolCount, &factorCount, &sourceCount, &noteCount)
	if err != nil {
		return nil, err
	}
	d.Breadth, err = loadBreadth(ctx, tx, breadth)
	if err != nil {
		return nil, err
	}
	d.Capacity, err = loadSealing(ctx, tx, capacity)
	if err != nil {
		return nil, err
	}
	d.Sealing, err = loadSealing(ctx, tx, sealing)
	if err != nil {
		return nil, err
	}
	d.Turnover, err = loadTurnoverView(ctx, tx, turnover)
	if err != nil {
		return nil, err
	}
	if top50 != nil {
		d.Top50Rebuild, err = loadTop50(ctx, tx, *top50)
		if err != nil {
			return nil, err
		}
	}
	if yesterday != nil {
		d.YesterdayRebuild, err = loadYesterday(ctx, tx, *yesterday)
		if err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT position,id,title,description,ready,count,valid_count,average,yang_rate,stocks FROM dashboard_pools WHERE dashboard=? ORDER BY position", id)
	if err != nil {
		return nil, err
	}
	sets := []int64{}
	for rows.Next() {
		var i int
		var set int64
		var p market.Pool
		if err := rows.Scan(&i, &p.ID, &p.Title, &p.Description, &p.Ready, &p.Count, &p.ValidCount, &p.Average, &p.YangRate, &set); err != nil {
			rows.Close()
			return nil, err
		}
		if i != len(d.Pools) {
			rows.Close()
			return nil, fmt.Errorf("pool position不连续")
		}
		sets = append(sets, set)
		d.Pools = append(d.Pools, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i, set := range sets {
		d.Pools[i].Stocks, err = loadStocks(ctx, tx, set)
		if err != nil {
			return nil, err
		}
	}
	rows, err = tx.QueryContext(ctx, "SELECT position,name,weight,value,score,unit,range_low,range_high FROM dashboard_factors WHERE dashboard=? ORDER BY position", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i int
		var f market.Factor
		if err := rows.Scan(&i, &f.Name, &f.Weight, &f.Value, &f.Score, &f.Unit, &f.Range[0], &f.Range[1]); err != nil {
			rows.Close()
			return nil, err
		}
		if i != len(d.Score.Factors) {
			rows.Close()
			return nil, fmt.Errorf("factor position不连续")
		}
		d.Score.Factors = append(d.Score.Factors, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT position,name,captured_at,source_time,provider,error,stale,fallback_count FROM dashboard_sources WHERE dashboard=? ORDER BY position", id)
	if err != nil {
		return nil, err
	}
	fallbackCounts := []int{}
	for rows.Next() {
		var i, count int
		s := market.SourceStatus{FallbackReasons: []string{}}
		if err := rows.Scan(&i, &s.Name, &s.CapturedAt, &s.SourceTime, &s.Provider, &s.Error, &s.Stale, &count); err != nil {
			rows.Close()
			return nil, err
		}
		if i != len(d.Sources) {
			rows.Close()
			return nil, fmt.Errorf("source position不连续")
		}
		d.Sources = append(d.Sources, s)
		fallbackCounts = append(fallbackCounts, count)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT source_position,position,reason FROM dashboard_source_fallbacks WHERE dashboard=? ORDER BY source_position,position", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i, j int
		var reason string
		if err := rows.Scan(&i, &j, &reason); err != nil {
			rows.Close()
			return nil, err
		}
		if i < 0 || i >= len(d.Sources) || j != len(d.Sources[i].FallbackReasons) {
			rows.Close()
			return nil, fmt.Errorf("source fallback position异常")
		}
		d.Sources[i].FallbackReasons = append(d.Sources[i].FallbackReasons, reason)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i, count := range fallbackCounts {
		if len(d.Sources[i].FallbackReasons) != count {
			return nil, fmt.Errorf("source fallback count不符")
		}
	}
	rows, err = tx.QueryContext(ctx, "SELECT position,note FROM dashboard_notes WHERE dashboard=? ORDER BY position", id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i int
		var note string
		if err := rows.Scan(&i, &note); err != nil {
			rows.Close()
			return nil, err
		}
		if i != len(d.Notes) {
			rows.Close()
			return nil, fmt.Errorf("note position不连续")
		}
		d.Notes = append(d.Notes, note)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(d.Pools) != poolCount || len(d.Score.Factors) != factorCount || len(d.Sources) != sourceCount || len(d.Notes) != noteCount {
		return nil, fmt.Errorf("dashboard 子记录数量不符")
	}
	return &d, validateDashboard(d)
}
