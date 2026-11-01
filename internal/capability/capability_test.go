package capability

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// fakeQuerier answers by exact query text; a missing query returns an error.
type fakeQuerier map[string]any // value: string, nil (SQL NULL) or error

func (f fakeQuerier) Scalar(_ context.Context, q string) (sql.NullString, error) {
	v, ok := f[q]
	if !ok {
		return sql.NullString{}, errors.New("unexpected query: " + q)
	}
	switch x := v.(type) {
	case string:
		return sql.NullString{String: x, Valid: true}, nil
	case error:
		return sql.NullString{}, x
	}
	return sql.NullString{}, nil
}

func TestDetect(t *testing.T) {
	denied := errors.New("SELECT command denied")
	tests := []struct {
		name   string
		q      fakeQuerier
		flavor Flavor
		perf   State
		user   State
		sys    State
		stmt   string
		// substrings that must appear in the hints of Unavailable/Degraded items
		hints map[string]string
	}{
		{
			name: "mariadb 11.4 everything on",
			q: fakeQuerier{qVersion: "11.4.2-MariaDB", qComment: "mariadb.org binary distribution",
				qPerfSchema: "1", qUserstat: "1", qSys: "1"},
			flavor: FlavorMariaDB, perf: Available, user: Available, sys: Available, stmt: "SHOW REPLICA STATUS",
		},
		{
			name: "mariadb 10.11 defaults: perf schema and userstat off",
			q: fakeQuerier{qVersion: "5.5.5-10.11.6-MariaDB-1:10.11.6+maria~ubu2204", qComment: "mariadb.org binary distribution",
				qPerfSchema: "0", qUserstat: "0", qSys: "1"},
			flavor: FlavorMariaDB, perf: Unavailable, user: Unavailable, sys: Degraded, stmt: "SHOW REPLICA STATUS",
			hints: map[string]string{"perf": "Enable performance_schema", "user": "Enable userstat", "sys": "needs performance_schema"},
		},
		{
			name:   "mariadb 10.5 predates sys schema",
			q:      fakeQuerier{qVersion: "10.5.9-MariaDB", qComment: "", qPerfSchema: "1", qUserstat: "0"},
			flavor: FlavorMariaDB, perf: Available, user: Unavailable, sys: Unavailable, stmt: "SHOW REPLICA STATUS",
			hints: map[string]string{"sys": "10.6"},
		},
		{
			name:   "mariadb 10.4 uses SLAVE",
			q:      fakeQuerier{qVersion: "10.4.30-MariaDB", qComment: "", qPerfSchema: "1", qUserstat: "1"},
			flavor: FlavorMariaDB, perf: Available, user: Available, sys: Unavailable, stmt: "SHOW SLAVE STATUS",
		},
		{
			name:   "mysql 8.4 performance_schema on, userstat n/a without querying it",
			q:      fakeQuerier{qVersion: "8.4.0", qComment: "MySQL Community Server - GPL", qPerfSchema: "1", qSys: "1"},
			flavor: FlavorMySQL, perf: Available, user: Unavailable, sys: Available, stmt: "SHOW REPLICA STATUS",
			hints: map[string]string{"user": "MariaDB feature"},
		},
		{
			name:   "mysql 8.0 uses SLAVE",
			q:      fakeQuerier{qVersion: "8.0.35-0ubuntu0.22.04.1", qComment: "(Ubuntu)", qPerfSchema: "1", qSys: "1"},
			flavor: FlavorMySQL, perf: Available, user: Unavailable, sys: Available, stmt: "SHOW SLAVE STATUS",
		},
		{
			name:   "mysql with performance_schema off and no sys",
			q:      fakeQuerier{qVersion: "8.4.0", qComment: "", qPerfSchema: "0", qSys: "0"},
			flavor: FlavorMySQL, perf: Unavailable, user: Unavailable, sys: Unavailable, stmt: "SHOW REPLICA STATUS",
			hints: map[string]string{"perf": "Enable performance_schema", "sys": "Install the sys schema"},
		},
		{
			name:   "percona style fork",
			q:      fakeQuerier{qVersion: "8.0.34-26", qComment: "Percona Server (GPL), Release 26", qPerfSchema: "1", qSys: "1"},
			flavor: FlavorMySQL, perf: Available, user: Unavailable, sys: Available, stmt: "SHOW SLAVE STATUS",
		},
		{
			name:   "probe errors become unavailable, not failures",
			q:      fakeQuerier{qVersion: "11.4.2-MariaDB", qComment: denied, qPerfSchema: denied, qUserstat: denied, qSys: denied},
			flavor: FlavorMariaDB, perf: Unavailable, user: Unavailable, sys: Unavailable, stmt: "SHOW REPLICA STATUS",
			hints: map[string]string{"perf": "denied", "user": "denied", "sys": "denied"},
		},
		{
			name:   "NULL perf schema value is unavailable",
			q:      fakeQuerier{qVersion: "8.4.0", qComment: "", qPerfSchema: nil, qSys: "1"},
			flavor: FlavorMySQL, perf: Unavailable, user: Unavailable, sys: Available, stmt: "SHOW REPLICA STATUS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Detect(context.Background(), tt.q)
			if err != nil {
				t.Fatal(err)
			}
			if r.Flavor != tt.flavor {
				t.Errorf("flavor = %s, want %s", r.Flavor, tt.flavor)
			}
			if r.PerformanceSchema.State != tt.perf {
				t.Errorf("performance_schema = %s, want %s", r.PerformanceSchema.State, tt.perf)
			}
			if r.Userstat.State != tt.user {
				t.Errorf("userstat = %s, want %s", r.Userstat.State, tt.user)
			}
			if r.Sys.State != tt.sys {
				t.Errorf("sys = %s, want %s", r.Sys.State, tt.sys)
			}
			if r.ReplicationStatement != tt.stmt {
				t.Errorf("statement = %q, want %q", r.ReplicationStatement, tt.stmt)
			}
			for which, c := range map[string]Capability{"perf": r.PerformanceSchema, "user": r.Userstat, "sys": r.Sys} {
				if c.State != Available && c.Hint == "" {
					t.Errorf("%s: non-available capability has no hint", which)
				}
				if c.State == Available && c.Hint != "" {
					t.Errorf("%s: available capability has hint %q", which, c.Hint)
				}
				if want, ok := tt.hints[which]; ok && !strings.Contains(c.Hint, want) {
					t.Errorf("%s hint %q does not contain %q", which, c.Hint, want)
				}
			}
		})
	}
}

func TestDetectFailures(t *testing.T) {
	tests := []struct {
		name string
		q    fakeQuerier
	}{
		{"version query fails", fakeQuerier{qVersion: errors.New("access denied")}},
		{"unparseable version", fakeQuerier{qVersion: "banana"}},
		{"NULL version", fakeQuerier{qVersion: nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if r, err := Detect(context.Background(), tt.q); err == nil {
				t.Fatalf("want error, got %+v", r)
			}
		})
	}
}

func TestReportText(t *testing.T) {
	r, err := Detect(context.Background(), fakeQuerier{qVersion: "8.4.0", qComment: "", qPerfSchema: "0", qSys: "0"})
	if err != nil {
		t.Fatal(err)
	}
	out := r.Text()
	for _, want := range []string{"flavor:", "mysql", "performance_schema:", "unavailable", "hint: Enable performance_schema", "SHOW REPLICA STATUS"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}
}
