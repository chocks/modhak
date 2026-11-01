package target

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// Session pins one connection so session-local settings apply to every query
// in a snapshot. Close it when done.
type Session struct {
	c    *Conn
	conn *sql.Conn
}

// Session checks out a dedicated connection.
func (c *Conn) Session(ctx context.Context) (*Session, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Target.ConnectTimeout)
	defer cancel()
	conn, err := c.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return &Session{c: c, conn: conn}, nil
}

// Close returns the connection to the pool.
func (s *Session) Close() error { return s.conn.Close() }

// Scanner is satisfied by *sql.Rows.
type Scanner interface{ Scan(dest ...any) error }

// QueryAll runs a SELECT or SHOW statement under the target's query timeout
// and calls scan for each row.
func (s *Session) QueryAll(ctx context.Context, query string, scan func(Scanner) error) error {
	if err := CheckReadOnly(query); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.c.queryTimeout)
	defer cancel()
	rows, err := s.conn.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// statsExpiryStmt makes information_schema.tables report current values
// (MySQL 8.0+ caches them for a day by default; see FEATURES.md V11). It is
// session-local and, like SET SESSION TRANSACTION READ ONLY, does not change
// data or schema.
const statsExpiryStmt = "SET SESSION information_schema_stats_expiry = 0"

// DisableStatsCache turns off the information_schema statistics cache for
// this session. Servers without the variable (MariaDB, MySQL 5.7) reject it
// with error 1193; that is not an error here.
func (s *Session) DisableStatsCache(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.c.queryTimeout)
	defer cancel()
	_, err := s.conn.ExecContext(ctx, statsExpiryStmt)
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1193 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("disable information_schema stats cache: %w", err)
	}
	return nil
}
