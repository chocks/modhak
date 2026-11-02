package scan

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/capability"
)

func sample() []TargetResult {
	days := 120.5
	return []TargetResult{
		{
			Target: "prod", Addr: "db:3306",
			Capabilities: &capability.Report{Flavor: capability.FlavorMariaDB, Version: "11.4.13"},
			Findings: []advisor.Finding{
				{ID: "auto_increment_headroom:prod:shop:orders:id", Kind: advisor.KindAutoIncHeadroom, Severity: advisor.SeverityCritical,
					Target:       advisor.Target{Name: "prod", Schema: "shop", Table: "orders", Column: "id"},
					Evidence:     advisor.AutoIncEvidence{ColumnType: "int(11)", Next: 2100000000, Max: 2147483647, UsedFraction: 0.978, Remaining: 47483648, RoughDaysToOverflow: &days, Note: "review before applying"},
					SuggestedSQL: []string{"ALTER TABLE `shop`.`orders` MODIFY `id` BIGINT NOT NULL AUTO_INCREMENT"}},
				{ID: "duplicate_index:prod:shop:customers:idx_email_dup", Kind: advisor.KindDuplicateIndex, Severity: advisor.SeverityWarning,
					Target:       advisor.Target{Name: "prod", Schema: "shop", Table: "customers", Index: "idx_email_dup"},
					Evidence:     advisor.DuplicateEvidence{Kept: "idx_email", Duplicate: "idx_email_dup", Columns: []advisor.IndexColumn{{Name: "email", SubPart: 10}}},
					SuggestedSQL: []string{"ALTER TABLE `shop`.`customers` DROP INDEX `idx_email_dup`"}},
				{ID: "redundant_index:prod:shop:orders:idx_c", Kind: advisor.KindRedundantIndex, Severity: advisor.SeverityWarning,
					Target:   advisor.Target{Name: "prod", Schema: "shop", Table: "orders", Index: "idx_c"},
					Evidence: advisor.RedundantEvidence{Redundant: "idx_c", CoveredBy: "idx_cs", Columns: []advisor.IndexColumn{{Name: "c"}}, CoveringBy: []advisor.IndexColumn{{Name: "c"}, {Name: "s", Desc: true}}}},
				{ID: "unused_index:prod:shop:orders:idx_z", Kind: advisor.KindUnusedIndex, Severity: advisor.SeverityInfo,
					Target:   advisor.Target{Name: "prod", Schema: "shop", Table: "orders", Index: "idx_z"},
					Evidence: advisor.UnusedEvidence{Source: advisor.SourcePerfSchema, ObservedFor: 30 * 24 * time.Hour, ObservedDays: 30, Caveat: "Observed for 30.0 days since the server started."}},
			},
		},
		{Target: "clean", Addr: "db2:3306", Capabilities: &capability.Report{Flavor: capability.FlavorMySQL, Version: "8.4.13"}, Findings: []advisor.Finding{},
			Skipped: []advisor.Skipped{{Analysis: advisor.SkipUnusedIndexes, Reason: "Enable performance_schema to get unused-index analysis."}}},
		{Target: "down", Addr: "db3:3306", Error: `target "down" (db3:3306): cannot connect`, Findings: []advisor.Finding{}},
	}
}

func TestText(t *testing.T) {
	out := Text(sample())
	for _, want := range []string{
		"== prod (db:3306)  mariadb 11.4.13",
		"4 findings",
		"[critical] auto_increment_headroom  shop.orders.id",
		"int(11): next value 2100000000 of 2147483647, 97.8% of the range used, 47483648 left",
		"rough estimate: about 120 days left",
		"suggested: ALTER TABLE `shop`.`orders` MODIFY `id` BIGINT NOT NULL AUTO_INCREMENT;",
		"[warning] duplicate_index  shop.customers.idx_email_dup",
		"idx_email_dup has the same columns (email(10)) as idx_email",
		"idx_c (c) is a left prefix of idx_cs (c, s DESC)",
		"[info] unused_index  shop.orders.idx_z",
		"0 reads (source: performance_schema)", "Observed for 30.0 days since the server started.",
		"== clean (db2:3306)  mysql 8.4.13",
		"no findings",
		"skipped unused_index_analysis: Enable performance_schema",
		"== down (db3:3306)",
		"error: target \"down\" (db3:3306): cannot connect",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q\n%s", want, out)
		}
	}
	if strings.Index(out, "[critical]") > strings.Index(out, "[warning]") {
		t.Error("critical should print before warning")
	}
}

func TestJSON(t *testing.T) {
	b, err := json.Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	var back []map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 3 {
		t.Fatalf("got %d targets", len(back))
	}
	first := back[0]["findings"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "kind", "severity", "target", "evidence", "suggested_sql"} {
		if _, ok := first[k]; !ok {
			t.Errorf("finding missing %q: %v", k, first)
		}
	}
	ev := first["evidence"].(map[string]any)
	if ev["max"].(float64) != 2147483647 || ev["rough_days_to_overflow"].(float64) != 120.5 {
		t.Errorf("evidence = %v", ev)
	}
	// Empty and failed targets must serialize findings as [], not null.
	for _, i := range []int{1, 2} {
		if f, ok := back[i]["findings"].([]any); !ok || len(f) != 0 {
			t.Errorf("target %d findings = %v", i, back[i]["findings"])
		}
	}
	if back[2]["error"] == nil || back[2]["capabilities"] != nil {
		t.Errorf("failed target = %v", back[2])
	}
	if back[1]["skipped"] == nil {
		t.Error("skipped missing")
	}
}

func TestRoughEstimateWording(t *testing.T) {
	mk := func(days float64) []TargetResult {
		r := sample()
		ev := r[0].Findings[0].Evidence.(advisor.AutoIncEvidence)
		ev.RoughDaysToOverflow = &days
		r[0].Findings[0].Evidence = ev
		return r[:1]
	}
	if out := Text(mk(0.4)); !strings.Contains(out, "less than a day left") {
		t.Errorf("sub-day estimate should say less than a day:\n%s", out)
	}
	if out := Text(mk(12)); !strings.Contains(out, "about 12 days left") {
		t.Errorf("estimate wording:\n%s", out)
	}
}
