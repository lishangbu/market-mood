package store

import (
	"context"
	"database/sql"
	"fmt"
	"market-mood/internal/catalog"
	"market-mood/internal/trading"
	"slices"
	"strings"
	"time"
)

const securityColumns = "code,name,exchange,board,industry,listing_date,listing_status,provider"

func scanSecurity(row interface{ Scan(...any) error }) (catalog.Security, error) {
	var s catalog.Security
	if err := row.Scan(&s.Code, &s.Name, &s.Exchange, &s.Board, &s.Industry, &s.ListingDate, &s.ListingStatus, &s.Provider); err != nil {
		return s, err
	}
	return s, s.Validate()
}
func (s *Store) ReplaceCatalog(ctx context.Context, result catalog.Result) error {
	counts, err := catalog.ValidateDirectory(result.Securities)
	if err != nil {
		return err
	}
	sources := []string{}
	for _, row := range result.Securities {
		if !slices.Contains(sources, row.Provider) {
			sources = append(sources, row.Provider)
		}
	}
	slices.Sort(sources)
	source := strings.Join(sources, "、")
	now := trading.Now().Format(time.RFC3339Nano)
	return s.transaction(ctx, "catalog", func(tx *sql.Tx) error {
		// Acquire the write lock before reading counts, avoiding deferred lock upgrades
		// racing another writer after a read snapshot has been opened.
		if _, err := tx.ExecContext(ctx, "UPDATE catalog_status SET attempted_at=attempted_at WHERE singleton=1"); err != nil {
			return err
		}
		for exchange, count := range counts {
			var old int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM securities WHERE exchange=?", exchange).Scan(&old); err != nil {
				return err
			}
			if count*100 < old*95 {
				return fmt.Errorf("股票目录数量异常下降，保留旧目录: %s", exchange)
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM securities"); err != nil {
			return err
		}
		statement, err := tx.PrepareContext(ctx, "INSERT INTO securities("+securityColumns+") VALUES(?,?,?,?,?,?,?,?)")
		if err != nil {
			return err
		}
		defer statement.Close()
		for _, row := range result.Securities {
			if _, err := statement.ExecContext(ctx, row.Code, row.Name, row.Exchange, row.Board, row.Industry, row.ListingDate, row.ListingStatus, row.Provider); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE catalog_status SET updated_at=?,attempted_at=?,provider=?,error=NULL WHERE singleton=1", now, now, source); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM catalog_fallbacks; DELETE FROM catalog_exchanges"); err != nil {
			return err
		}
		for i, reason := range result.FallbackReasons {
			if _, err := tx.ExecContext(ctx, "INSERT INTO catalog_fallbacks VALUES(?,?)", i, reason); err != nil {
				return err
			}
		}
		for exchange, count := range counts {
			var check *string
			if v, ok := result.ExchangeChecks[exchange]; ok {
				check = &v
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO catalog_exchanges VALUES(?,?,?)", exchange, count, check); err != nil {
				return err
			}
		}
		inserted, err := tx.ExecContext(ctx, "INSERT INTO catalog_revisions(captured_at,provider) VALUES(?,?)", now, source)
		if err != nil {
			return err
		}
		revision, err := inserted.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO catalog_revision_securities SELECT ?,"+securityColumns+" FROM securities", revision); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO catalog_revision_checks SELECT ?,exchange,count,check_result FROM catalog_exchanges", revision); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO catalog_revision_fallbacks SELECT ?,position,reason FROM catalog_fallbacks", revision)
		return err
	})
}
func (s *Store) SaveCatalogError(ctx context.Context, message string) (resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "SaveCatalogError")
	if acquireErr != nil {
		return s.wrap("SaveCatalogError", acquireErr)
	}
	defer finish(&resultErr)
	_, err := conn.ExecContext(ctx, "UPDATE catalog_status SET attempted_at=?,error=? WHERE singleton=1", trading.Now().Format(time.RFC3339Nano), message)
	return s.wrap("catalog_status", err)
}
func catalogStatus(ctx context.Context, tx *sql.Tx) (catalog.Status, error) {
	status := catalog.Status{FallbackReasons: []string{}, Counts: map[string]int{}, ExchangeChecks: map[string]string{}}
	if err := tx.QueryRowContext(ctx, "SELECT updated_at,attempted_at,provider,error FROM catalog_status WHERE singleton=1").Scan(&status.UpdatedAt, &status.AttemptedAt, &status.Provider, &status.Error); err != nil {
		return status, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT reason FROM catalog_fallbacks ORDER BY position")
	if err != nil {
		return status, err
	}
	for rows.Next() {
		var reason string
		if err := rows.Scan(&reason); err != nil {
			rows.Close()
			return status, err
		}
		status.FallbackReasons = append(status.FallbackReasons, reason)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return status, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT exchange,count,check_result FROM catalog_exchanges")
	if err != nil {
		return status, err
	}
	defer rows.Close()
	for rows.Next() {
		var exchange string
		var count int
		var check *string
		if err := rows.Scan(&exchange, &count, &check); err != nil {
			return status, err
		}
		status.Counts[exchange] = count
		if check != nil {
			status.ExchangeChecks[exchange] = *check
		}
	}
	return status, rows.Err()
}
func (s *Store) CatalogStatus(ctx context.Context) (_ catalog.Status, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "CatalogStatus")
	if acquireErr != nil {
		return catalog.Status{}, s.wrap("CatalogStatus", acquireErr)
	}
	defer finish(&resultErr)
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return catalog.Status{}, s.wrap("catalog_status", err)
	}
	defer tx.Rollback()
	status, err := catalogStatus(ctx, tx)
	if err != nil {
		return status, s.wrap("catalog_status", err)
	}
	return status, s.wrap("catalog_status", tx.Commit())
}
func (s *Store) AllSecurities(ctx context.Context) (_ []catalog.Security, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "AllSecurities")
	if acquireErr != nil {
		return nil, s.wrap("AllSecurities", acquireErr)
	}
	defer finish(&resultErr)
	rows, err := conn.QueryContext(ctx, "SELECT "+securityColumns+" FROM securities ORDER BY code")
	if err != nil {
		return nil, s.wrap("securities", err)
	}
	defer rows.Close()
	items := []catalog.Security{}
	for rows.Next() {
		item, err := scanSecurity(rows)
		if err != nil {
			return nil, s.wrap("securities/"+item.Code, err)
		}
		items = append(items, item)
	}
	return items, s.wrap("securities", rows.Err())
}
func (s *Store) Security(ctx context.Context, code string) (_ *catalog.Security, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Security")
	if acquireErr != nil {
		return nil, s.wrap("Security", acquireErr)
	}
	defer finish(&resultErr)
	item, err := scanSecurity(conn.QueryRowContext(ctx, "SELECT "+securityColumns+" FROM securities WHERE code=?", code))
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, s.wrap("securities/"+code, err)
	}
	return &item, nil
}
func (s *Store) Securities(ctx context.Context, query catalog.Query) (_ catalog.Page, resultErr error) {
	conn, finish, acquireErr := s.acquire(ctx, "Securities")
	if acquireErr != nil {
		return catalog.Page{}, s.wrap("Securities", acquireErr)
	}
	defer finish(&resultErr)
	out := catalog.Page{Items: []catalog.Security{}, Page: query.Page, PageSize: query.PageSize}
	if err := query.Validate(); err != nil {
		return out, err
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, s.wrap("securities", err)
	}
	defer tx.Rollback()
	search := strings.TrimSpace(query.Q)
	// instr uses literal substring matching; % and _ in user input aren't wildcards.
	filter := " WHERE (?='' OR instr(code,?)>0 OR instr(lower(name),lower(?))>0) AND (?='' OR exchange=?)"
	args := []any{search, search, search, query.Exchange, query.Exchange}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM securities"+filter, args...).Scan(&out.Total); err != nil {
		return out, s.wrap("securities/count", err)
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := tx.QueryContext(ctx, "SELECT "+securityColumns+" FROM securities"+filter+" ORDER BY code LIMIT ? OFFSET ?", args...)
	if err != nil {
		return out, s.wrap("securities/page", err)
	}
	for rows.Next() {
		item, err := scanSecurity(rows)
		if err != nil {
			rows.Close()
			return out, s.wrap("securities/"+item.Code, err)
		}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, s.wrap("securities/page", err)
	}
	out.Status, err = catalogStatus(ctx, tx)
	if err != nil {
		return out, s.wrap("catalog_status", err)
	}
	return out, s.wrap("securities", tx.Commit())
}
