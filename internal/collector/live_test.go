package collector

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chocks/modhak/internal/advisor"
	"github.com/chocks/modhak/internal/capability"
	"github.com/chocks/modhak/internal/config"
	"github.com/chocks/modhak/internal/target"
)

// Live test: skipped unless MODHAK_TEST_ADDR (host:port) is set. Expects the
// docker compose dev server, whose docker/seed.sql has a duplicate index
// (customers.idx_email_dup) and a redundant one (orders.idx_customer).
// Flavor-agnostic: runs on MariaDB and MySQL.
func TestLiveSnapshotAndAnalyze(t *testing.T) {
	addr := os.Getenv("MODHAK_TEST_ADDR")
	if addr == "" {
		t.Skip("MODHAK_TEST_ADDR not set")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	c, err := target.Open(config.Target{Name: "live", Host: host, Port: port, User: "modhak", Password: "modhak",
		ConnectTimeout: 5 * time.Second, QueryTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()

	rep, err := capability.Detect(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := c.Session(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	in, err := Snapshot(ctx, sess, "live", rep, time.Now())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	for _, ix := range in.Indexes {
		switch ix.Schema {
		case "mysql", "information_schema", "performance_schema", "sys":
			t.Fatalf("system schema leaked: %+v", ix)
		}
	}

	// Default config: a freshly started dev server is below the 7-day minimum.
	res := advisor.Analyze(in, advisor.Config{})
	has := func(fs []advisor.Finding, id string) *advisor.Finding {
		for i := range fs {
			if fs[i].ID == id {
				return &fs[i]
			}
		}
		return nil
	}
	if has(res.Findings, "duplicate_index:live:shop:customers:idx_email_dup") == nil {
		t.Errorf("missing duplicate finding; got %v", findingIDs(res.Findings))
	}
	if has(res.Findings, "redundant_index:live:shop:orders:idx_customer") == nil {
		t.Errorf("missing redundant finding; got %v", findingIDs(res.Findings))
	}
	if rep.IndexUsage.State == capability.Available {
		if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "below the minimum") {
			t.Errorf("fresh server should skip unused-index verdicts for short uptime: %+v", res.Skipped)
		}
		// With a tiny minimum the same data yields unused-index findings.
		loose := advisor.Analyze(in, advisor.Config{MinUptime: time.Second})
		for _, sk := range loose.Skipped {
			if strings.Contains(sk.Reason, "minimum") {
				t.Errorf("unexpected uptime skip: %+v", sk)
			}
		}
		for _, f := range loose.Findings {
			if f.Kind == advisor.KindUnusedIndex && f.Target.Index == "PRIMARY" {
				t.Errorf("primary key reported unused: %+v", f)
			}
		}
	} else if len(res.Skipped) != 1 || res.Skipped[0].Reason == "" {
		t.Errorf("without index usage the analysis must be skipped with a reason: %+v", res.Skipped)
	}
	for _, f := range res.Findings {
		for _, q := range f.SuggestedSQL {
			if !strings.HasPrefix(q, "ALTER TABLE ") {
				t.Errorf("unexpected suggested SQL %q", q)
			}
		}
	}
	t.Logf("%d indexes, %d auto-increment columns, usage=%v, findings=%v", len(in.Indexes), len(in.AutoIncrements), in.Usage != nil, findingIDs(res.Findings))
}

func findingIDs(fs []advisor.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.ID)
	}
	return out
}
