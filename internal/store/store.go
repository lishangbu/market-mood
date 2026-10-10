// Package store persists typed records through explicit SQLite queries and transactions.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed schema.sql
var schema string

const applicationID = 1296910160 // Identifies databases owned by this application.
const schemaVersion = 1

type Store struct {
	db      *sql.DB
	path    string
	metrics telemetry
}

func Open(ctx context.Context, path string) (*Store, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0750); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	query := url.Values{"_pragma": {"foreign_keys(1)", "busy_timeout(5000)"}}
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	s := &Store{db: db, path: absolute, metrics: telemetry{started: time.Now(), operations: map[string]Operation{}}}
	if err := s.initialize(ctx); err != nil {
		db.Close()
		return nil, s.wrap("schema", err)
	}
	return s, nil
}
func (s *Store) initialize(ctx context.Context) error {
	var id, version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&id); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if id != 0 && id != applicationID {
		return fmt.Errorf("application_id=%d 非 Go 版本数据库，不自动迁移", id)
	}
	if version != 0 && version != schemaVersion {
		return fmt.Errorf("不支持的数据库版本 %d", version)
	}
	if id == 0 || version == 0 {
		var tables int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return fmt.Errorf("未标识的非空数据库，不自动改写")
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id=%d", applicationID)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	return err
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Path() string { return s.path }
func (s *Store) wrap(record string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("数据库 %s 记录 %s: %w", s.path, record, err)
}
func (s *Store) transaction(ctx context.Context, record string, write func(*sql.Tx) error) (resultErr error) {
	name, _, _ := strings.Cut(record, "/")
	conn, finish, err := s.acquire(ctx, "write_"+name)
	if err != nil {
		return s.wrap(record, err)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return s.wrap(record, err)
	}
	defer tx.Rollback()
	if err := write(tx); err != nil {
		return s.wrap(record, err)
	}
	return s.wrap(record, tx.Commit())
}
func validDay(day string) error { _, err := time.Parse(time.DateOnly, day); return err }
func missing(err error) bool    { return errors.Is(err, sql.ErrNoRows) }
