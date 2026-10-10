package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/market"
	"market-mood/internal/provider"
	"market-mood/internal/trading"
	"strings"
	"time"
)

func (s *Store) Watchlist(ctx context.Context) (_ []market.Watch, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Watchlist")
	if acquireErr != nil {
		return nil, s.wrap("Watchlist", acquireErr)
	}
	defer finish(&resultErr)
	rows, err := conn.QueryContext(ctx, "SELECT code,name,created_at FROM watchlist ORDER BY created_at DESC,code")
	if err != nil {
		return nil, s.wrap("watchlist", err)
	}
	defer rows.Close()
	out := []market.Watch{}
	for rows.Next() {
		var w market.Watch
		if err := rows.Scan(&w.Code, &w.Name, &w.CreatedAt); err != nil {
			return nil, s.wrap("watchlist", err)
		}
		out = append(out, w)
	}
	return out, s.wrap("watchlist", rows.Err())
}
func (s *Store) Watch(ctx context.Context, code, name string) (resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Watch")
	if acquireErr != nil {
		return s.wrap("Watch", acquireErr)
	}
	defer finish(&resultErr)
	if !provider.ValidCode(code) || strings.TrimSpace(name) == "" || len(name) > 240 {
		return fmt.Errorf("自选代码或名称无效")
	}
	_, err := conn.ExecContext(ctx, "INSERT INTO watchlist(code,name,created_at) VALUES(?,?,?) ON CONFLICT(code) DO UPDATE SET name=excluded.name", code, name, trading.Now().Format(time.RFC3339Nano))
	return s.wrap("watchlist/"+code, err)
}
func (s *Store) Unwatch(ctx context.Context, code string) (resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Unwatch")
	if acquireErr != nil {
		return s.wrap("Unwatch", acquireErr)
	}
	defer finish(&resultErr)
	_, err := conn.ExecContext(ctx, "DELETE FROM watchlist WHERE code=?", code)
	return s.wrap("watchlist/"+code, err)
}
func (s *Store) SaveConfig(ctx context.Context, config provider.Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	return s.transaction(ctx, "source_config", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM source_config"); err != nil {
			return err
		}
		for capability, list := range map[string][]provider.Source{"quotes": config.Quotes, "stock_quotes": config.StockQuotes} {
			for i, source := range list {
				if _, err := tx.ExecContext(ctx, "INSERT INTO source_config VALUES(?,?,?)", capability, i, source); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func (s *Store) Config(ctx context.Context) (_ *provider.Config, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Config")
	if acquireErr != nil {
		return nil, s.wrap("Config", acquireErr)
	}
	defer finish(&resultErr)
	rows, err := conn.QueryContext(ctx, "SELECT capability,source FROM source_config ORDER BY capability,position")
	if err != nil {
		return nil, s.wrap("source_config", err)
	}
	defer rows.Close()
	var config provider.Config
	count := 0
	for rows.Next() {
		var capability string
		var source provider.Source
		if err := rows.Scan(&capability, &source); err != nil {
			return nil, s.wrap("source_config", err)
		}
		if capability == "quotes" {
			config.Quotes = append(config.Quotes, source)
		} else {
			config.StockQuotes = append(config.StockQuotes, source)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, s.wrap("source_config", err)
	}
	if count == 0 {
		return nil, nil
	}
	if err := config.Validate(); err != nil {
		return nil, s.wrap("source_config", err)
	}
	return &config, nil
}
