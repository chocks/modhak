// Package collector reads a target's catalogs and turns them into advisor
// input. M1 has only the one-shot Snapshot used by `modhak scan`; scheduled
// collection and history arrive in M2.
//
// Every catalog read here is gated on the capability report (ADR 0004), and
// every query is a SELECT or SHOW (enforced in internal/target).
package collector

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/capability"
	"github.com/chocks/modhak/internal/target"
)

// Source runs read-only queries on one pinned session. *target.Session
// implements it.
type Source interface {
	QueryAll(ctx context.Context, query string, scan func(target.Scanner) error) error
	DisableStatsCache(ctx context.Context) error
}

// systemSchemas are never analyzed.
const systemSchemas = "('mysql','information_schema','performance_schema','sys')"

// Snapshot reads indexes, AUTO_INCREMENT columns and, when the capability
// layer says index usage is trustworthy, index read counters and uptime.
// name is the target name used in findings; now is passed to the advisor.
func Snapshot(ctx context.Context, src Source, name string, rep *capability.Report, now time.Time) (advisor.Input, error) {
	in := advisor.Input{Target: name, Now: now}
	info, err := capability.ParseServerInfo(rep.Raw, rep.Comment)
	if err != nil {
		return in, err
	}
	if info.Flavor == capability.FlavorMySQL {
		if err := src.DisableStatsCache(ctx); err != nil {
			return in, err
		}
	}
	if in.Indexes, err = readIndexes(ctx, src, info); err != nil {
		return in, fmt.Errorf("read indexes: %w", err)
	}
	if in.AutoIncrements, err = readAutoIncrements(ctx, src, now); err != nil {
		return in, fmt.Errorf("read AUTO_INCREMENT columns: %w", err)
	}
	if rep.IndexUsage.State == capability.Available && rep.IndexUsageSource == "performance_schema" {
		if in.Usage, err = readUsage(ctx, src); err != nil {
			return in, fmt.Errorf("read index usage: %w", err)
		}
	} else {
		in.UsageUnavailable = rep.IndexUsage.Hint
	}
	return in, nil
}

// indexesQuery selects the extra visibility columns the flavor and version
// are known to have (FEATURES.md V11: MariaDB 10.6+ IGNORED; MySQL 8.0+
// IS_VISIBLE and EXPRESSION). Older servers are analyzed without them.
func indexesQuery(info capability.ServerInfo) (query string, extra int) {
	cols := "table_schema, table_name, index_name, non_unique, seq_in_index, column_name, `collation`, sub_part, index_type"
	switch {
	case info.Flavor == capability.FlavorMariaDB && info.Version.AtLeast(10, 6, 0):
		cols += ", ignored"
		extra = 1
	case info.Flavor == capability.FlavorMySQL && info.Version.AtLeast(8, 0, 0):
		cols += ", is_visible, expression"
		extra = 2
	}
	return "SELECT " + cols + " FROM information_schema.statistics WHERE table_schema NOT IN " + systemSchemas +
		" ORDER BY table_schema, table_name, index_name, seq_in_index", extra
}

type indexRow struct {
	schema, table, index string
	nonUnique            int
	column               sql.NullString
	collation            sql.NullString
	subPart              sql.NullInt64
	indexType            string
	ignored              bool
	expression           string
}

func readIndexes(ctx context.Context, src Source, info capability.ServerInfo) ([]advisor.Index, error) {
	query, extra := indexesQuery(info)
	var rows []indexRow
	err := src.QueryAll(ctx, query, func(sc target.Scanner) error {
		var r indexRow
		var seq int
		dest := []any{&r.schema, &r.table, &r.index, &r.nonUnique, &seq, &r.column, &r.collation, &r.subPart, &r.indexType}
		var flag, expr sql.NullString
		switch extra {
		case 1:
			dest = append(dest, &flag)
		case 2:
			dest = append(dest, &flag, &expr)
		}
		if err := sc.Scan(dest...); err != nil {
			return err
		}
		switch extra {
		case 1: // MariaDB IGNORED = 'YES'
			r.ignored = strings.EqualFold(flag.String, "YES")
		case 2: // MySQL IS_VISIBLE = 'NO'
			r.ignored = strings.EqualFold(flag.String, "NO")
			r.expression = expr.String
		}
		rows = append(rows, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return groupIndexes(rows), nil
}

// groupIndexes folds per-column rows (ordered by table, index, position)
// into indexes.
func groupIndexes(rows []indexRow) []advisor.Index {
	var out []advisor.Index
	for _, r := range rows {
		if n := len(out); n == 0 || out[n-1].Schema != r.schema || out[n-1].Table != r.table || out[n-1].Name != r.index {
			out = append(out, advisor.Index{
				Schema: r.schema, Table: r.table, Name: r.index,
				Unique: r.nonUnique == 0, Type: r.indexType, Ignored: r.ignored,
			})
		}
		ix := &out[len(out)-1]
		ix.Columns = append(ix.Columns, advisor.IndexColumn{
			Name:       r.column.String,
			SubPart:    int(r.subPart.Int64),
			Desc:       r.collation.String == "D",
			Expression: r.expression,
		})
	}
	return out
}

// AUTO_INCREMENT columns joined with the table's next value and age. The age
// is computed on the server (TIMESTAMPDIFF against NOW()) so server time zone
// does not skew the rough days-to-overflow.
const autoIncrementQuery = "SELECT c.table_schema, c.table_name, c.column_name, c.column_type, t.auto_increment, " +
	"TIMESTAMPDIFF(SECOND, t.create_time, NOW()) " +
	"FROM information_schema.columns c JOIN information_schema.tables t " +
	"ON t.table_schema = c.table_schema AND t.table_name = c.table_name " +
	"WHERE c.extra LIKE '%auto_increment%' AND t.table_type = 'BASE TABLE' AND t.auto_increment IS NOT NULL " +
	"AND c.table_schema NOT IN " + systemSchemas +
	" ORDER BY c.table_schema, c.table_name, c.column_name"

func readAutoIncrements(ctx context.Context, src Source, now time.Time) ([]advisor.AutoIncrement, error) {
	var out []advisor.AutoIncrement
	err := src.QueryAll(ctx, autoIncrementQuery, func(sc target.Scanner) error {
		var a advisor.AutoIncrement
		var age sql.NullInt64
		if err := sc.Scan(&a.Schema, &a.Table, &a.Column, &a.ColumnType, &a.Next, &age); err != nil {
			return err
		}
		if age.Valid && age.Int64 > 0 {
			a.CreateTime = now.Add(-time.Duration(age.Int64) * time.Second)
		}
		out = append(out, a)
		return nil
	})
	return out, err
}

const (
	usageQuery = "SELECT object_schema, object_name, index_name, count_read " +
		"FROM performance_schema.table_io_waits_summary_by_index_usage " +
		"WHERE object_type = 'TABLE' AND index_name IS NOT NULL AND object_schema NOT IN " + systemSchemas +
		" ORDER BY object_schema, object_name, index_name"
	uptimeQuery = "SHOW GLOBAL STATUS LIKE 'Uptime'"
)

func readUsage(ctx context.Context, src Source) (*advisor.Usage, error) {
	u := &advisor.Usage{Source: advisor.SourcePerfSchema}
	var uptimeSeen bool
	err := src.QueryAll(ctx, uptimeQuery, func(sc target.Scanner) error {
		var name, value string
		if err := sc.Scan(&name, &value); err != nil {
			return err
		}
		secs, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("parse Uptime %q: %w", value, err)
		}
		u.Uptime = time.Duration(secs) * time.Second
		uptimeSeen = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !uptimeSeen {
		return nil, fmt.Errorf("server did not report Uptime")
	}
	err = src.QueryAll(ctx, usageQuery, func(sc target.Scanner) error {
		var e advisor.IndexUsage
		if err := sc.Scan(&e.Schema, &e.Table, &e.Index, &e.Reads); err != nil {
			return err
		}
		u.Entries = append(u.Entries, e)
		return nil
	})
	return u, err
}
