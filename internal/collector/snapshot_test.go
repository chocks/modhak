package collector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/capability"
	"github.com/chocks/modhak/internal/target"
)

var now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

// fakeScanner copies canned values into Scan destinations.
type fakeScanner []any

func (f fakeScanner) Scan(dest ...any) error {
	if len(dest) != len(f) {
		return fmt.Errorf("scan: %d destinations for %d values", len(dest), len(f))
	}
	for i, d := range dest {
		v := f[i]
		switch p := d.(type) {
		case *sql.NullString:
			if v == nil {
				*p = sql.NullString{}
			} else {
				*p = sql.NullString{String: v.(string), Valid: true}
			}
		case *sql.NullInt64:
			if v == nil {
				*p = sql.NullInt64{}
			} else {
				*p = sql.NullInt64{Int64: int64(v.(int)), Valid: true}
			}
		default:
			if v == nil {
				return fmt.Errorf("NULL into %T", d)
			}
			dv := reflect.ValueOf(d).Elem()
			sv := reflect.ValueOf(v)
			if !sv.Type().ConvertibleTo(dv.Type()) {
				return fmt.Errorf("cannot convert %T to %s", v, dv.Type())
			}
			dv.Set(sv.Convert(dv.Type()))
		}
	}
	return nil
}

// fakeSource answers queries by a distinguishing substring and records them.
type fakeSource struct {
	answers    map[string][]fakeScanner
	queries    []string
	statsReset int
	statsErr   error
}

func (f *fakeSource) QueryAll(_ context.Context, q string, scan func(target.Scanner) error) error {
	if err := target.CheckReadOnly(q); err != nil {
		return err
	}
	f.queries = append(f.queries, q)
	for key, rows := range f.answers {
		if strings.Contains(q, key) {
			for _, r := range rows {
				if err := scan(r); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return fmt.Errorf("unexpected query: %s", q)
}

func (f *fakeSource) DisableStatsCache(context.Context) error { f.statsReset++; return f.statsErr }

func (f *fakeSource) ran(sub string) bool {
	for _, q := range f.queries {
		if strings.Contains(q, sub) {
			return true
		}
	}
	return false
}

func report(version, comment string, usage capability.State, source string) *capability.Report {
	return &capability.Report{Raw: version, Comment: comment,
		IndexUsage:       capability.Capability{State: usage, Hint: map[bool]string{true: "", false: "Enable performance_schema to get unused-index analysis."}[usage == capability.Available]},
		IndexUsageSource: source}
}

func TestIndexesQueryByFlavor(t *testing.T) {
	tests := []struct {
		name, version, comment string
		wantCols               []string
		avoidCols              []string
		extra                  int
	}{
		{"mariadb 11.4", "11.4.13-MariaDB-ubu2404", "", []string{", ignored FROM"}, []string{"is_visible", "expression"}, 1},
		{"mariadb 10.6", "10.6.28-MariaDB-ubu2204", "", []string{", ignored FROM"}, []string{"is_visible"}, 1},
		{"mariadb 10.5 has no IGNORED", "10.5.9-MariaDB", "", nil, []string{"ignored", "is_visible"}, 0},
		{"mysql 8.4", "8.4.13", "", []string{", is_visible, expression FROM"}, []string{"ignored"}, 2},
		{"mysql 8.0", "8.0.46", "", []string{", is_visible, expression FROM"}, []string{"ignored"}, 2},
		{"mysql 5.7 has neither", "5.7.44-log", "", nil, []string{"ignored", "is_visible", "expression"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := capability.ParseServerInfo(tt.version, tt.comment)
			if err != nil {
				t.Fatal(err)
			}
			q, extra := indexesQuery(info)
			if extra != tt.extra {
				t.Errorf("extra = %d, want %d", extra, tt.extra)
			}
			for _, w := range tt.wantCols {
				if !strings.Contains(q, w) {
					t.Errorf("query missing %q: %s", w, q)
				}
			}
			for _, a := range tt.avoidCols {
				if strings.Contains(q, a) {
					t.Errorf("query should not contain %q: %s", a, q)
				}
			}
			if !strings.Contains(q, "'mysql','information_schema','performance_schema','sys'") {
				t.Errorf("system schemas not excluded: %s", q)
			}
			if err := target.CheckReadOnly(q); err != nil {
				t.Errorf("query is not read-only: %v", err)
			}
		})
	}
}

func TestSnapshotMariaDB(t *testing.T) {
	src := &fakeSource{answers: map[string][]fakeScanner{
		"FROM information_schema.statistics": {
			// schema, table, index, non_unique, seq, column, collation, sub_part, type, ignored
			{"shop", "orders", "PRIMARY", 0, 1, "id", "A", nil, "BTREE", "NO"},
			{"shop", "orders", "idx_cs", 1, 1, "customer_id", "A", nil, "BTREE", "NO"},
			{"shop", "orders", "idx_cs", 1, 2, "status", "D", nil, "BTREE", "NO"},
			{"shop", "orders", "idx_old", 1, 1, "note", "A", 20, "BTREE", "YES"},
		},
		"FROM information_schema.columns": {
			{"shop", "orders", "id", "bigint(20) unsigned", uint64(5), 86400},
			{"shop", "legacy", "id", "int(11)", uint64(7), nil},
		},
		"SHOW GLOBAL STATUS": {{"Uptime", "864000"}},
		"performance_schema.table_io_waits_summary_by_index_usage": {
			{"shop", "orders", "PRIMARY", uint64(10)},
			{"shop", "orders", "idx_cs", uint64(0)},
		},
	}}
	in, err := Snapshot(context.Background(), src, "prod", report("11.4.13-MariaDB-ubu2404", "", capability.Available, "performance_schema"), now)
	if err != nil {
		t.Fatal(err)
	}
	if src.statsReset != 0 {
		t.Errorf("MariaDB should not touch information_schema_stats_expiry")
	}
	wantIdx := []advisor.Index{
		{Schema: "shop", Table: "orders", Name: "PRIMARY", Unique: true, Type: "BTREE", Columns: []advisor.IndexColumn{{Name: "id"}}},
		{Schema: "shop", Table: "orders", Name: "idx_cs", Type: "BTREE", Columns: []advisor.IndexColumn{{Name: "customer_id"}, {Name: "status", Desc: true}}},
		{Schema: "shop", Table: "orders", Name: "idx_old", Type: "BTREE", Ignored: true, Columns: []advisor.IndexColumn{{Name: "note", SubPart: 20}}},
	}
	if !reflect.DeepEqual(in.Indexes, wantIdx) {
		t.Errorf("indexes:\n got %+v\nwant %+v", in.Indexes, wantIdx)
	}
	if len(in.AutoIncrements) != 2 || in.AutoIncrements[0].Next != 5 || !in.AutoIncrements[0].CreateTime.Equal(now.Add(-24*time.Hour)) {
		t.Errorf("auto increments = %+v", in.AutoIncrements)
	}
	if !in.AutoIncrements[1].CreateTime.IsZero() {
		t.Errorf("NULL create_time should leave CreateTime zero: %+v", in.AutoIncrements[1])
	}
	if in.Usage == nil || in.Usage.Uptime != 864000*time.Second || in.Usage.Source != advisor.SourcePerfSchema || len(in.Usage.Entries) != 2 {
		t.Errorf("usage = %+v", in.Usage)
	}
	if in.Target != "prod" || !in.Now.Equal(now) {
		t.Errorf("target/now = %q %v", in.Target, in.Now)
	}
}

func TestSnapshotMySQLResetsStatsCacheAndReadsVisibility(t *testing.T) {
	src := &fakeSource{answers: map[string][]fakeScanner{
		"FROM information_schema.statistics": {
			{"shop", "t", "PRIMARY", 0, 1, "id", "A", nil, "BTREE", "YES", nil},
			{"shop", "t", "hidden", 1, 1, "a", "A", nil, "BTREE", "NO", nil},
			{"shop", "t", "fn", 1, 1, nil, "A", nil, "BTREE", "YES", "(`a` + `b`)"},
		},
		"FROM information_schema.columns": {},
	}}
	in, err := Snapshot(context.Background(), src, "m", report("8.4.13", "", capability.Unavailable, ""), now)
	if err != nil {
		t.Fatal(err)
	}
	if src.statsReset != 1 {
		t.Errorf("stats cache reset %d times, want 1", src.statsReset)
	}
	if !in.Indexes[1].Ignored || in.Indexes[0].Ignored {
		t.Errorf("IS_VISIBLE=NO should mean ignored: %+v", in.Indexes)
	}
	if got := in.Indexes[2].Columns[0]; got.Expression != "(`a` + `b`)" || got.Name != "" {
		t.Errorf("functional column = %+v", got)
	}
}

func TestSnapshotDoesNotReadUsageUnlessCapabilityAllows(t *testing.T) {
	answers := map[string][]fakeScanner{"FROM information_schema.statistics": {}, "FROM information_schema.columns": {}}
	tests := []struct {
		name string
		rep  *capability.Report
		hint string
	}{
		{"unavailable", report("11.4.13-MariaDB", "", capability.Unavailable, ""), "Enable performance_schema"},
		{"available but not perf schema source", report("11.4.13-MariaDB", "", capability.Available, "userstat"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fakeSource{answers: answers}
			in, err := Snapshot(context.Background(), src, "t", tt.rep, now)
			if err != nil {
				t.Fatal(err)
			}
			if src.ran("table_io_waits_summary_by_index_usage") || src.ran("SHOW GLOBAL STATUS") {
				t.Fatalf("usage queries ran despite capability: %v", src.queries)
			}
			if in.Usage != nil || !strings.Contains(in.UsageUnavailable, tt.hint) {
				t.Fatalf("usage = %+v, unavailable = %q", in.Usage, in.UsageUnavailable)
			}
			// The advisor must then report the analysis as skipped with that reason.
			res := advisor.Analyze(in, advisor.Config{})
			if len(res.Skipped) != 1 || res.Skipped[0].Analysis != advisor.SkipUnusedIndexes {
				t.Fatalf("skipped = %+v", res.Skipped)
			}
			if in.UsageUnavailable != "" && res.Skipped[0].Reason != in.UsageUnavailable {
				t.Fatalf("skip reason %q should be the capability hint %q", res.Skipped[0].Reason, in.UsageUnavailable)
			}
		})
	}
}

func TestSnapshotErrors(t *testing.T) {
	ok := map[string][]fakeScanner{"FROM information_schema.statistics": {}, "FROM information_schema.columns": {}}
	t.Run("unparseable version", func(t *testing.T) {
		if _, err := Snapshot(context.Background(), &fakeSource{answers: ok}, "t", report("banana", "", capability.Unavailable, ""), now); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("stats cache failure", func(t *testing.T) {
		src := &fakeSource{answers: ok, statsErr: errors.New("boom")}
		if _, err := Snapshot(context.Background(), src, "t", report("8.4.13", "", capability.Unavailable, ""), now); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("missing uptime", func(t *testing.T) {
		a := map[string][]fakeScanner{"FROM information_schema.statistics": {}, "FROM information_schema.columns": {}, "SHOW GLOBAL STATUS": {}}
		_, err := Snapshot(context.Background(), &fakeSource{answers: a}, "t", report("11.4.13-MariaDB", "", capability.Available, "performance_schema"), now)
		if err == nil || !strings.Contains(err.Error(), "Uptime") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad uptime value", func(t *testing.T) {
		a := map[string][]fakeScanner{"FROM information_schema.statistics": {}, "FROM information_schema.columns": {}, "SHOW GLOBAL STATUS": {{"Uptime", "abc"}}}
		_, err := Snapshot(context.Background(), &fakeSource{answers: a}, "t", report("11.4.13-MariaDB", "", capability.Available, "performance_schema"), now)
		if err == nil {
			t.Fatal("want error")
		}
	})
}
