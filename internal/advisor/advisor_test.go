package advisor

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func cols(names ...string) []IndexColumn {
	var out []IndexColumn
	for _, n := range names {
		out = append(out, IndexColumn{Name: n})
	}
	return out
}

func ix(table, name string, unique bool, c ...string) Index {
	return Index{Schema: "shop", Table: table, Name: name, Unique: unique, Type: "BTREE", Columns: cols(c...)}
}

func ids(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}

func TestDuplicateIndexes(t *testing.T) {
	ignored := ix("t", "idx_b", false, "a")
	ignored.Ignored = true
	prefix := ix("t", "idx_pre", false, "name")
	prefix.Columns[0].SubPart = 10
	full := ix("t", "idx_full", false, "name")
	desc := ix("t", "idx_desc", false, "a")
	desc.Columns[0].Desc = true
	fulltext1 := ix("t", "ft1", false, "body")
	fulltext1.Type = "FULLTEXT"
	fulltext2 := ix("t", "ft2", false, "body")
	fulltext2.Type = "FULLTEXT"
	fn1 := Index{Schema: "shop", Table: "t", Name: "fn1", Type: "BTREE", Columns: []IndexColumn{{Expression: "lower(a)"}}}
	fn2 := fn1
	fn2.Name = "fn2"

	tests := []struct {
		name    string
		indexes []Index
		wantIDs []string
		wantSQL []string
	}{
		{"none", []Index{ix("t", "a", false, "x"), ix("t", "b", false, "y")}, nil, nil},
		{"two identical keep first by name", []Index{ix("t", "idx_b", false, "a"), ix("t", "idx_a", false, "a")},
			[]string{"duplicate_index:srv:shop:t:idx_b"}, []string{"ALTER TABLE `shop`.`t` DROP INDEX `idx_b`"}},
		{"three identical", []Index{ix("t", "i1", false, "a"), ix("t", "i2", false, "a"), ix("t", "i3", false, "a")},
			[]string{"duplicate_index:srv:shop:t:i2", "duplicate_index:srv:shop:t:i3"}, nil},
		{"unique twin: drop the non-unique", []Index{ix("t", "idx_a", false, "a"), ix("t", "uq_a", true, "a")},
			[]string{"duplicate_index:srv:shop:t:idx_a"}, []string{"ALTER TABLE `shop`.`t` DROP INDEX `idx_a`"}},
		{"duplicate of primary: drop secondary", []Index{ix("t", "PRIMARY", true, "id"), ix("t", "idx_id", false, "id")},
			[]string{"duplicate_index:srv:shop:t:idx_id"}, []string{"ALTER TABLE `shop`.`t` DROP INDEX `idx_id`"}},
		{"two unique duplicates: no SQL", []Index{ix("t", "uq1", true, "a"), ix("t", "uq2", true, "a")},
			[]string{"duplicate_index:srv:shop:t:uq2"}, []string{}},
		{"different order is not duplicate", []Index{ix("t", "i1", false, "a", "b"), ix("t", "i2", false, "b", "a")}, nil, nil},
		{"different prefix length is not duplicate", []Index{prefix, full}, nil, nil},
		{"descending differs", []Index{ix("t", "i1", false, "a"), desc}, nil, nil},
		{"visible vs ignored twin skipped", []Index{ix("t", "idx_a", false, "a"), ignored}, nil, nil},
		{"fulltext skipped", []Index{fulltext1, fulltext2}, nil, nil},
		{"functional skipped", []Index{fn1, fn2}, nil, nil},
		{"same name different tables", []Index{ix("t1", "i", false, "a"), ix("t2", "i", false, "a")}, nil, nil},
		{"backtick in name is quoted", []Index{ix("t", "a", false, "x"), ix("t", "b`c", false, "x")},
			[]string{"duplicate_index:srv:shop:t:b`c"}, []string{"ALTER TABLE `shop`.`t` DROP INDEX `b``c`"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := DuplicateIndexes(Input{Target: "srv", Indexes: tt.indexes})
			sortFindings(fs)
			if got := ids(fs); !reflect.DeepEqual(got, tt.wantIDs) && !(len(got) == 0 && len(tt.wantIDs) == 0) {
				t.Fatalf("ids = %v, want %v", got, tt.wantIDs)
			}
			if tt.wantSQL != nil && len(fs) > 0 {
				var sql []string
				for _, f := range fs {
					sql = append(sql, f.SuggestedSQL...)
				}
				if len(sql) != len(tt.wantSQL) || (len(sql) > 0 && !reflect.DeepEqual(sql, tt.wantSQL)) {
					t.Fatalf("sql = %v, want %v", sql, tt.wantSQL)
				}
			}
		})
	}
}

func TestRedundantIndexes(t *testing.T) {
	ignoredCover := ix("t", "idx_ab", false, "a", "b")
	ignoredCover.Ignored = true
	subA := ix("t", "idx_a10", false, "a")
	subA.Columns[0].SubPart = 10
	tests := []struct {
		name      string
		indexes   []Index
		wantIDs   []string
		coveredBy string
	}{
		{"prefix of composite", []Index{ix("t", "idx_a", false, "a"), ix("t", "idx_ab", false, "a", "b")}, []string{"redundant_index:srv:shop:t:idx_a"}, "idx_ab"},
		{"covered by unique composite", []Index{ix("t", "idx_a", false, "a"), ix("t", "uq_ab", true, "a", "b")}, []string{"redundant_index:srv:shop:t:idx_a"}, "uq_ab"},
		{"covered by primary", []Index{ix("t", "PRIMARY", true, "a", "b"), ix("t", "idx_a", false, "a")}, []string{"redundant_index:srv:shop:t:idx_a"}, "PRIMARY"},
		{"not a prefix", []Index{ix("t", "idx_b", false, "b"), ix("t", "idx_ab", false, "a", "b")}, nil, ""},
		{"unique prefix is kept", []Index{ix("t", "uq_a", true, "a"), ix("t", "idx_ab", false, "a", "b")}, nil, ""},
		{"primary is kept", []Index{ix("t", "PRIMARY", true, "a"), ix("t", "idx_ab", false, "a", "b")}, nil, ""},
		{"ignored cover does not count", []Index{ix("t", "idx_a", false, "a"), ignoredCover}, nil, ""},
		{"prefix-length mismatch is not covered", []Index{subA, ix("t", "idx_ab", false, "a", "b")}, nil, ""},
		{"equal columns are duplicates not redundant", []Index{ix("t", "i1", false, "a"), ix("t", "i2", false, "a")}, nil, ""},
		{"picks primary over others as cover", []Index{ix("t", "idx_a", false, "a"), ix("t", "z_ab", false, "a", "b"), ix("t", "PRIMARY", true, "a", "c")}, []string{"redundant_index:srv:shop:t:idx_a"}, "PRIMARY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := RedundantIndexes(Input{Target: "srv", Indexes: tt.indexes})
			if got := ids(fs); len(got)+len(tt.wantIDs) > 0 && !reflect.DeepEqual(got, tt.wantIDs) {
				t.Fatalf("ids = %v, want %v", got, tt.wantIDs)
			}
			if len(fs) == 1 {
				if ev := fs[0].Evidence.(RedundantEvidence); ev.CoveredBy != tt.coveredBy {
					t.Fatalf("covered by %q, want %q", ev.CoveredBy, tt.coveredBy)
				}
				if len(fs[0].SuggestedSQL) != 1 || !strings.Contains(fs[0].SuggestedSQL[0], "DROP INDEX") {
					t.Fatalf("sql = %v", fs[0].SuggestedSQL)
				}
			}
		})
	}
}

func TestUnusedIndexes(t *testing.T) {
	indexes := []Index{
		ix("t", "PRIMARY", true, "id"),
		ix("t", "uq", true, "u"),
		ix("t", "idx_used", false, "a"),
		ix("t", "idx_zero", false, "b"),
		ix("t", "idx_unknown", false, "c"),
	}
	ig := ix("t", "idx_ignored", false, "d")
	ig.Ignored = true
	indexes = append(indexes, ig)
	usage := func(up time.Duration) *Usage {
		return &Usage{Source: SourcePerfSchema, Uptime: up, Entries: []IndexUsage{
			{"shop", "t", "PRIMARY", 0}, {"shop", "t", "uq", 0}, {"shop", "t", "idx_used", 5},
			{"shop", "t", "idx_zero", 0}, {"shop", "t", "idx_ignored", 0},
		}}
	}
	day := 24 * time.Hour
	tests := []struct {
		name        string
		usage       *Usage
		cfg         Config
		wantIDs     []string
		wantSkipped string
	}{
		{"zero reads past min uptime", usage(30 * day), Config{}, []string{"unused_index:srv:shop:t:idx_zero"}, ""},
		{"below default min uptime: no verdict", usage(2 * day), Config{}, nil, "below the minimum"},
		{"custom min uptime allows verdict", usage(2 * day), Config{MinUptime: day}, []string{"unused_index:srv:shop:t:idx_zero"}, ""},
		{"exactly min uptime counts", usage(7 * day), Config{}, []string{"unused_index:srv:shop:t:idx_zero"}, ""},
		{"no usage data", nil, Config{}, nil, "enable userstat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, sk := UnusedIndexes(Input{Target: "srv", Indexes: indexes, Usage: tt.usage}, tt.cfg)
			if got := ids(fs); len(got)+len(tt.wantIDs) > 0 && !reflect.DeepEqual(got, tt.wantIDs) {
				t.Fatalf("ids = %v, want %v", got, tt.wantIDs)
			}
			if tt.wantSkipped == "" {
				if sk != nil {
					t.Fatalf("unexpected skip: %+v", sk)
				}
			} else if sk == nil || !strings.Contains(sk.Reason, tt.wantSkipped) || sk.Analysis != SkipUnusedIndexes {
				t.Fatalf("skip = %+v, want reason containing %q", sk, tt.wantSkipped)
			}
			for _, f := range fs {
				ev := f.Evidence.(UnusedEvidence)
				if ev.Source != SourcePerfSchema || ev.Reads != 0 || ev.ObservedDays <= 0 {
					t.Errorf("bad evidence %+v", ev)
				}
				if !strings.Contains(ev.Caveat, "Observed for") || !strings.Contains(ev.Caveat, "days") {
					t.Errorf("caveat missing observation window: %q", ev.Caveat)
				}
				if len(f.SuggestedSQL) != 1 {
					t.Errorf("sql = %v", f.SuggestedSQL)
				}
			}
		})
	}
}

func TestAutoIncrementHeadroom(t *testing.T) {
	created := now.Add(-100 * 24 * time.Hour)
	mk := func(colType string, next uint64) AutoIncrement {
		return AutoIncrement{Schema: "shop", Table: "t", Column: "id", ColumnType: colType, Next: next, CreateTime: created}
	}
	tests := []struct {
		name     string
		ai       AutoIncrement
		cfg      Config
		wantSev  Severity // "" means no finding
		wantMax  uint64
		wantSQL  string
		wantDays bool
	}{
		{"int unsigned low", mk("int(10) unsigned", 1000), Config{}, "", 0, "", false},
		{"int unsigned at 50% is below warn", mk("int unsigned", 2147483648), Config{}, "", 0, "", false},
		{"int unsigned 70% warns", mk("int(10) unsigned", 3000000000), Config{}, SeverityWarning, 4294967295, "MODIFY `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT", true},
		{"int signed 90% critical", mk("int(11)", 1932735284), Config{}, SeverityCritical, 2147483647, "MODIFY `id` BIGINT NOT NULL AUTO_INCREMENT", true},
		{"tinyint signed critical", mk("tinyint(4)", 120), Config{}, SeverityCritical, 127, "MODIFY `id` SMALLINT NOT NULL AUTO_INCREMENT", true},
		{"mediumint unsigned", mk("mediumint(8) unsigned zerofill", 16000000), Config{}, SeverityCritical, 16777215, "MODIFY `id` INT UNSIGNED NOT NULL AUTO_INCREMENT", true},
		{"bigint critical has no SQL", mk("bigint unsigned", 18000000000000000000), Config{}, SeverityCritical, 18446744073709551615, "", true},
		{"table full clamps to 100%", mk("tinyint unsigned", 256), Config{}, SeverityCritical, 255, "MODIFY `id` SMALLINT UNSIGNED NOT NULL AUTO_INCREMENT", true},
		{"custom thresholds", mk("int unsigned", 2147483648), Config{HeadroomWarn: 0.4}, SeverityWarning, 4294967295, "BIGINT UNSIGNED", true},
		{"non-integer type skipped", mk("decimal(10,0)", 99999999), Config{}, "", 0, "", false},
		{"next of zero skipped", mk("int", 0), Config{}, "", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := AutoIncrementHeadroom(Input{Target: "srv", Now: now, AutoIncrements: []AutoIncrement{tt.ai}}, tt.cfg)
			if tt.wantSev == "" {
				if len(fs) != 0 {
					t.Fatalf("unexpected findings: %+v", fs)
				}
				return
			}
			if len(fs) != 1 {
				t.Fatalf("got %d findings", len(fs))
			}
			f := fs[0]
			ev := f.Evidence.(AutoIncEvidence)
			if f.Severity != tt.wantSev || ev.Max != tt.wantMax {
				t.Fatalf("severity %s max %d, want %s %d", f.Severity, ev.Max, tt.wantSev, tt.wantMax)
			}
			if (ev.RoughDaysToOverflow != nil) != tt.wantDays {
				t.Fatalf("rough days = %v, want present=%v", ev.RoughDaysToOverflow, tt.wantDays)
			}
			if tt.wantSQL == "" {
				if len(f.SuggestedSQL) != 0 {
					t.Fatalf("unexpected sql %v", f.SuggestedSQL)
				}
			} else if len(f.SuggestedSQL) != 1 || !strings.Contains(f.SuggestedSQL[0], tt.wantSQL) {
				t.Fatalf("sql = %v, want containing %q", f.SuggestedSQL, tt.wantSQL)
			}
		})
	}
}

func TestAutoIncrementRoughDays(t *testing.T) {
	// 3,000,000,000 of 4,294,967,295 used in 100 days: 30M/day, 1,294,967,296 left.
	ai := AutoIncrement{Schema: "s", Table: "t", Column: "id", ColumnType: "int unsigned", Next: 3000000001, CreateTime: now.Add(-100 * 24 * time.Hour)}
	fs := AutoIncrementHeadroom(Input{Target: "srv", Now: now, AutoIncrements: []AutoIncrement{ai}}, Config{})
	got := *fs[0].Evidence.(AutoIncEvidence).RoughDaysToOverflow
	want := 1294967295.0 / 30000000.0
	if got < want-0.01 || got > want+0.01 {
		t.Fatalf("rough days = %v, want ~%v", got, want)
	}
}

func TestAnalyzeSortedDeterministicAndJSON(t *testing.T) {
	in := Input{
		Target: "srv", Now: now,
		Indexes: []Index{
			ix("orders", "idx_c", false, "c"), ix("orders", "idx_c2", false, "c"),
			ix("orders", "idx_a", false, "a"), ix("orders", "idx_ab", false, "a", "b"),
			ix("orders", "PRIMARY", true, "id"), ix("orders", "idx_zero", false, "z"),
		},
		Usage: &Usage{Source: SourceUserstat, Uptime: 30 * 24 * time.Hour,
			Entries: []IndexUsage{{"shop", "orders", "idx_zero", 0}, {"shop", "orders", "idx_c", 3}}},
		AutoIncrements: []AutoIncrement{{Schema: "shop", Table: "orders", Column: "id", ColumnType: "int", Next: 2100000000}},
	}
	first := Analyze(in, Config{})
	if len(first.Findings) != 4 {
		t.Fatalf("want 4 findings, got %v", ids(first.Findings))
	}
	if first.Findings[0].Kind != KindAutoIncHeadroom || first.Findings[0].Severity != SeverityCritical {
		t.Fatalf("critical finding should sort first: %v", ids(first.Findings))
	}
	// Reordering the input must not change the output.
	rev := in
	rev.Indexes = append([]Index(nil), in.Indexes...)
	for i, j := 0, len(rev.Indexes)-1; i < j; i, j = i+1, j-1 {
		rev.Indexes[i], rev.Indexes[j] = rev.Indexes[j], rev.Indexes[i]
	}
	second := Analyze(rev, Config{})
	if !reflect.DeepEqual(ids(first.Findings), ids(second.Findings)) {
		t.Fatalf("not deterministic:\n%v\n%v", ids(first.Findings), ids(second.Findings))
	}
	b, err := json.Marshal(first)
	if err != nil || !strings.Contains(string(b), `"kind":"auto_increment_headroom"`) {
		t.Fatalf("json: %v %s", err, b)
	}
}

func TestAnalyzeReportsSkippedWithoutUsage(t *testing.T) {
	res := Analyze(Input{Target: "srv", Now: now, Indexes: []Index{ix("t", "i", false, "a")}}, Config{})
	if len(res.Skipped) != 1 || res.Skipped[0].Analysis != SkipUnusedIndexes {
		t.Fatalf("skipped = %+v", res.Skipped)
	}
}
