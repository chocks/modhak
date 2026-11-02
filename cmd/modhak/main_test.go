package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "modhak.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRun(t *testing.T) {
	// Port 1 on loopback refuses connections quickly; no database needed.
	unreachable := writeConfig(t, "targets:\n  - {name: down1, host: 127.0.0.1, port: 1, user: u, connect_timeout: 1s}\n  - {name: down2, host: 127.0.0.1, port: 2, user: u, connect_timeout: 1s}\n")
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string
		wantStderr []string
	}{
		{"no args", nil, 2, nil, []string{"Usage:"}},
		{"unknown command", []string{"nope"}, 2, nil, []string{`unknown command "nope"`}},
		{"version", []string{"version"}, 0, []string{"modhak dev"}, nil},
		{"missing config", []string{"capabilities", "--config", "/nonexistent.yaml"}, 2, nil, []string{"open config"}},
		{"unknown target", []string{"capabilities", "--config", unreachable, "--target", "zzz"}, 2, nil, []string{`no target named "zzz"`}},
		{"all targets attempted when unreachable", []string{"capabilities", "--config", unreachable}, 1,
			[]string{"== down1", "== down2", `target "down1"`, `target "down2"`, "cannot connect"}, nil},
		{"json output", []string{"capabilities", "--config", unreachable, "--target", "down1", "--json"}, 1,
			[]string{`"target": "down1"`, `"error"`}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := run(context.Background(), tt.args, &out, &errb)
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.wantCode, out.String(), errb.String())
			}
			for _, w := range tt.wantStdout {
				if !strings.Contains(out.String(), w) {
					t.Errorf("stdout missing %q:\n%s", w, out.String())
				}
			}
			for _, w := range tt.wantStderr {
				if !strings.Contains(errb.String(), w) {
					t.Errorf("stderr missing %q:\n%s", w, errb.String())
				}
			}
		})
	}
}

func TestScanUsageAndFailures(t *testing.T) {
	unreachable := writeConfig(t, "targets:\n  - {name: down1, host: 127.0.0.1, port: 1, user: u, connect_timeout: 1s}\n  - {name: down2, host: 127.0.0.1, port: 2, user: u, connect_timeout: 1s}\n")
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string
		wantStderr []string
	}{
		{"missing config", []string{"scan", "--config", "/nonexistent.yaml"}, 2, nil, []string{"open config"}},
		{"unknown target", []string{"scan", "--config", unreachable, "--target", "zzz"}, 2, nil, []string{`no target named "zzz"`}},
		{"bad flag", []string{"scan", "--nope"}, 2, nil, []string{"flag provided but not defined"}},
		{"warn above critical", []string{"scan", "--config", unreachable, "--headroom-warn", "0.9", "--headroom-critical", "0.5"}, 2, nil, []string{"headroom-warn"}},
		{"negative min uptime", []string{"scan", "--config", unreachable, "--min-uptime", "-1h"}, 2, nil, []string{"min-uptime"}},
		{"all targets attempted", []string{"scan", "--config", unreachable}, 1,
			[]string{"== down1", "== down2", `target "down1"`, `target "down2"`, "cannot connect"}, nil},
		{"json", []string{"scan", "--config", unreachable, "--target", "down1", "--json"}, 1,
			[]string{`"target": "down1"`, `"error"`, `"findings": []`}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := run(context.Background(), tt.args, &out, &errb)
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.wantCode, out.String(), errb.String())
			}
			for _, w := range tt.wantStdout {
				if !strings.Contains(out.String(), w) {
					t.Errorf("stdout missing %q:\n%s", w, out.String())
				}
			}
			for _, w := range tt.wantStderr {
				if !strings.Contains(errb.String(), w) {
					t.Errorf("stderr missing %q:\n%s", w, errb.String())
				}
			}
		})
	}
}

// Live test: skipped unless MODHAK_TEST_ADDR (host:port) is set. Expects the
// docker compose dev server and its shop seed. Works on MariaDB and MySQL.
func TestScanLive(t *testing.T) {
	addr := os.Getenv("MODHAK_TEST_ADDR")
	if addr == "" {
		t.Skip("MODHAK_TEST_ADDR not set")
	}
	host, port, _ := strings.Cut(addr, ":")
	cfg := writeConfig(t, "targets:\n  - {name: live, host: "+host+", port: "+port+", user: modhak, password: modhak}\n")

	t.Run("text", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := run(context.Background(), []string{"scan", "--config", cfg}, &out, &errb); code != 0 {
			t.Fatalf("code %d\n%s\n%s", code, out.String(), errb.String())
		}
		for _, want := range []string{"== live", "[warning] duplicate_index  shop.customers.idx_email_dup",
			"suggested: ALTER TABLE `shop`.`customers` DROP INDEX `idx_email_dup`;",
			"[warning] redundant_index  shop.orders.idx_customer", "skipped unused_index_analysis"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("missing %q\n%s", want, out.String())
			}
		}
	})
	t.Run("json with no minimum uptime", func(t *testing.T) {
		var out, errb bytes.Buffer
		if code := run(context.Background(), []string{"scan", "--config", cfg, "--json", "--min-uptime", "0"}, &out, &errb); code != 0 {
			t.Fatalf("code %d\n%s\n%s", code, out.String(), errb.String())
		}
		var res []struct {
			Target   string `json:"target"`
			Error    string `json:"error"`
			Findings []struct {
				ID, Kind, Severity string
				SuggestedSQL       []string `json:"suggested_sql"`
			} `json:"findings"`
			Skipped []struct{ Analysis, Reason string } `json:"skipped"`
		}
		if err := json.Unmarshal(out.Bytes(), &res); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if len(res) != 1 || res[0].Error != "" {
			t.Fatalf("unexpected result: %+v", res)
		}
		// Indexes on tables not accessed since startup (MySQL) get a note, never
		// a missing-minimum-uptime skip.
		for _, sk := range res[0].Skipped {
			if strings.Contains(sk.Reason, "minimum") {
				t.Errorf("min-uptime 0 should not skip for uptime: %+v", sk)
			}
		}
		kinds := map[string]bool{}
		for _, f := range res[0].Findings {
			kinds[f.Kind] = true
			for _, q := range f.SuggestedSQL {
				if !strings.HasPrefix(q, "ALTER TABLE ") {
					t.Errorf("unexpected SQL %q", q)
				}
			}
		}
		// unused_index appears only for indexes that have counters: always on
		// MariaDB, and on MySQL only once the tables have been accessed.
		for _, k := range []string{"duplicate_index", "redundant_index"} {
			if !kinds[k] {
				t.Errorf("no %s finding; kinds = %v", k, kinds)
			}
		}
		if !kinds["unused_index"] && len(res[0].Skipped) == 0 {
			t.Errorf("no unused_index finding and no note explaining why; kinds = %v", kinds)
		}
	})
}
