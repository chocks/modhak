package config

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string // substring; empty means success
		check   func(t *testing.T, c *Config)
	}{
		{
			name: "defaults applied",
			in:   "targets:\n  - {name: a, host: h, user: u}\n",
			check: func(t *testing.T, c *Config) {
				g := c.Targets[0]
				if g.Port != 3306 || g.ConnectTimeout != 5*time.Second || g.QueryTimeout != 10*time.Second {
					t.Fatalf("defaults not applied: %+v", g)
				}
			},
		},
		{
			name: "explicit values and two targets",
			in:   "targets:\n  - {name: a, host: h, port: 3307, user: u, connect_timeout: 1s, query_timeout: 2s, tls: Skip-Verify}\n  - {name: b, host: h2, user: u}\n",
			check: func(t *testing.T, c *Config) {
				a := c.Targets[0]
				if a.Port != 3307 || a.ConnectTimeout != time.Second || a.QueryTimeout != 2*time.Second || a.TLS != "skip-verify" {
					t.Fatalf("unexpected: %+v", a)
				}
				if got, ok := c.Target("b"); !ok || got.Host != "h2" {
					t.Fatalf("lookup b: %+v %v", got, ok)
				}
			},
		},
		{name: "empty file", in: "", wantErr: "empty"},
		{name: "no targets", in: "targets: []\n", wantErr: "no targets"},
		{name: "missing name", in: "targets:\n  - {host: h, user: u}\n", wantErr: "name is required"},
		{name: "bad name", in: "targets:\n  - {name: 'a/b', host: h, user: u}\n", wantErr: "name must match"},
		{name: "duplicate", in: "targets:\n  - {name: a, host: h, user: u}\n  - {name: a, host: h, user: u}\n", wantErr: "duplicate"},
		{name: "missing host", in: "targets:\n  - {name: a, user: u}\n", wantErr: "host is required"},
		{name: "missing user", in: "targets:\n  - {name: a, host: h}\n", wantErr: "user is required"},
		{name: "port range", in: "targets:\n  - {name: a, host: h, user: u, port: 70000}\n", wantErr: "out of range"},
		{name: "both passwords", in: "targets:\n  - {name: a, host: h, user: u, password: x, password_env: Y}\n", wantErr: "only one of"},
		{name: "bad tls", in: "targets:\n  - {name: a, host: h, user: u, tls: maybe}\n", wantErr: "tls"},
		{name: "negative timeout", in: "targets:\n  - {name: a, host: h, user: u, query_timeout: -1s}\n", wantErr: "negative"},
		{name: "unknown field", in: "targets:\n  - {name: a, host: h, user: u, hots: x}\n", wantErr: "hots"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse(strings.NewReader(tt.in))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, c)
		})
	}
}

func TestResolvedPassword(t *testing.T) {
	env := func(k string) string {
		if k == "PW" {
			return "secret"
		}
		return ""
	}
	tests := []struct {
		name    string
		t       Target
		want    string
		wantErr bool
	}{
		{"literal", Target{Name: "a", Password: "p"}, "p", false},
		{"none", Target{Name: "a"}, "", false},
		{"env", Target{Name: "a", PasswordEnv: "PW"}, "secret", false},
		{"env unset", Target{Name: "a", PasswordEnv: "NOPE"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.t.ResolvedPassword(env)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}
