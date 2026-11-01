// Package target manages connections to the databases modhak watches.
//
// Sessions are read-only and every query runs under a timeout. modhak never
// executes DDL against a target (AGENTS.md, ADR 0007); the only statement
// this package issues that is not a SELECT is the session-local
// SET SESSION TRANSACTION READ ONLY that enforces that rule.
package target

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/chocks/modhak/internal/config"
	"github.com/go-sql-driver/mysql"
)

// readOnlyStmt makes every new session reject writes, as defense in depth on
// top of a read-only DB user.
const readOnlyStmt = "SET SESSION TRANSACTION READ ONLY"

// Conn is a connection pool to one target. It implements capability.Querier.
type Conn struct {
	Target       config.Target
	db           *sql.DB
	queryTimeout time.Duration
}

// Open prepares a pool for t. No connection is made until the first query.
func Open(t config.Target) (*Conn, error) {
	pw, err := t.ResolvedPassword(os.Getenv)
	if err != nil {
		return nil, err
	}
	cfg := mysql.NewConfig()
	cfg.User = t.User
	cfg.Passwd = pw
	cfg.Net = "tcp"
	cfg.Addr = t.Addr()
	cfg.Timeout = t.ConnectTimeout
	cfg.ReadTimeout = t.QueryTimeout
	cfg.WriteTimeout = t.QueryTimeout
	if t.TLS != "" {
		cfg.TLSConfig = t.TLS
	}
	base, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("target %q: %w", t.Name, err)
	}
	db := sql.OpenDB(readOnlyConnector{base})
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &Conn{Target: t, db: db, queryTimeout: t.QueryTimeout}, nil
}

// Close releases the pool.
func (c *Conn) Close() error { return c.db.Close() }

// Scalar runs a query expected to return one row and one column.
func (c *Conn) Scalar(ctx context.Context, query string) (sql.NullString, error) {
	if err := CheckReadOnly(query); err != nil {
		return sql.NullString{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout+c.Target.ConnectTimeout)
	defer cancel()
	var v sql.NullString
	if err := c.db.QueryRowContext(ctx, query).Scan(&v); err != nil {
		return sql.NullString{}, err
	}
	return v, nil
}

// readOnlyConnector sets the session read-only on every new connection.
type readOnlyConnector struct{ driver.Connector }

func (r readOnlyConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := r.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	ex, ok := conn.(driver.ExecerContext)
	if !ok {
		conn.Close()
		return nil, errors.New("driver connection cannot execute statements")
	}
	if _, err := ex.ExecContext(ctx, readOnlyStmt, nil); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set read-only session: %w", err)
	}
	return conn, nil
}

// Explain turns a driver or network error into a short message that names the
// target and says what to check.
func Explain(t config.Target, err error) error {
	if err == nil {
		return nil
	}
	var me *mysql.MySQLError
	var ne net.Error
	switch {
	case errors.As(err, &me) && (me.Number == 1045 || me.Number == 1044 || me.Number == 1698):
		return fmt.Errorf("target %q (%s): access denied for user %q; check user, password and host grants (%v)", t.Name, t.Addr(), t.User, me.Message)
	case errors.As(err, &me):
		return fmt.Errorf("target %q (%s): server error %d: %s", t.Name, t.Addr(), me.Number, me.Message)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("target %q (%s): timed out after connect_timeout=%s / query_timeout=%s; check host, port and firewall", t.Name, t.Addr(), t.ConnectTimeout, t.QueryTimeout)
	case errors.As(err, &ne) && ne.Timeout():
		return fmt.Errorf("target %q (%s): connection timed out after %s; check host, port and firewall", t.Name, t.Addr(), t.ConnectTimeout)
	case errors.As(err, new(*net.OpError)):
		return fmt.Errorf("target %q (%s): cannot connect: %v; is the server running and reachable?", t.Name, t.Addr(), err)
	}
	return fmt.Errorf("target %q (%s): %w", t.Name, t.Addr(), err)
}
