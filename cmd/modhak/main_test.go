package main

import (
	"bytes"
	"context"
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
