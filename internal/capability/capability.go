// Package capability detects what a target database supports (ADR 0004).
//
// Features must ask this package before reading a catalog or running
// SHOW ... STATUS, and handle Degraded / Unavailable by showing the hint.
// Anything that cannot be confirmed counts as Unavailable.
package capability

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// State is the availability of one capability.
type State int

const (
	Unavailable State = iota
	Degraded
	Available
)

func (s State) String() string {
	switch s {
	case Available:
		return "available"
	case Degraded:
		return "degraded"
	default:
		return "unavailable"
	}
}

func (s State) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// Capability is one detected feature. Hint is empty when Available; otherwise
// it says what to enable to get the feature, or why it is limited.
type Capability struct {
	State State  `json:"state"`
	Hint  string `json:"hint,omitempty"`
}

// Report is the detection result for one target. Because it is also data for
// the UI and API (ADR 0004), it is JSON-serializable.
type Report struct {
	Flavor  Flavor `json:"flavor"`
	Version string `json:"version"`
	Raw     string `json:"raw_version"`
	Comment string `json:"version_comment,omitempty"`

	PerformanceSchema Capability `json:"performance_schema"`
	Userstat          Capability `json:"userstat"`
	Sys               Capability `json:"sys_schema"`
	Replication       Capability `json:"replication"`
	// ReplicationStatement is the statement to use for replication status,
	// e.g. "SHOW REPLICA STATUS". Empty unless Replication is Available.
	ReplicationStatement string `json:"replication_statement,omitempty"`
}

// Querier runs a single-value query. Implementations enforce timeouts and
// the read-only session; detection issues only SELECT.
type Querier interface {
	Scalar(ctx context.Context, query string) (sql.NullString, error)
}

// Detection queries. Matrix rows in docs/FEATURES.md mark most of these
// UNVERIFIED until the checklist is run against real servers.
const (
	qVersion    = "SELECT VERSION()"
	qComment    = "SELECT @@version_comment"
	qPerfSchema = "SELECT @@performance_schema"
	qUserstat   = "SELECT @@userstat"
	qSys        = "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = 'sys'"
)

// Detect probes the target. It returns an error only if the flavor and
// version cannot be determined (e.g. unreachable or unauthorized); a failing
// individual probe yields an Unavailable capability instead.
func Detect(ctx context.Context, q Querier) (*Report, error) {
	ver, err := q.Scalar(ctx, qVersion)
	if err != nil {
		return nil, fmt.Errorf("read server version: %w", err)
	}
	// @@version_comment only refines the flavor; ignore failure.
	comment, _ := q.Scalar(ctx, qComment)
	info, err := ParseServerInfo(ver.String, comment.String)
	if err != nil {
		return nil, err
	}

	r := &Report{
		Flavor:  info.Flavor,
		Version: info.Version.String(),
		Raw:     info.Raw,
		Comment: info.Comment,
	}
	r.PerformanceSchema = detectPerfSchema(ctx, q)
	r.Userstat = detectUserstat(ctx, q, info)
	r.Sys = detectSys(ctx, q, info, r.PerformanceSchema)
	r.Replication, r.ReplicationStatement = replicationStatement(info)
	return r, nil
}

func isTrue(v sql.NullString) bool {
	switch strings.ToUpper(v.String) {
	case "1", "ON", "TRUE":
		return v.Valid
	}
	return false
}

func probeFailed(what string, err error) Capability {
	return Capability{Unavailable, fmt.Sprintf("could not check %s: %v", what, err)}
}

func detectPerfSchema(ctx context.Context, q Querier) Capability {
	v, err := q.Scalar(ctx, qPerfSchema)
	if err != nil {
		return probeFailed("performance_schema", err)
	}
	if !isTrue(v) {
		return Capability{Unavailable, "Enable performance_schema (set performance_schema=ON in the server config and restart) to get query digests and index usage."}
	}
	return Capability{State: Available}
}

func detectUserstat(ctx context.Context, q Querier, info ServerInfo) Capability {
	if info.Flavor != FlavorMariaDB {
		return Capability{Unavailable, "userstat is a MariaDB feature; use performance_schema for index usage on MySQL."}
	}
	v, err := q.Scalar(ctx, qUserstat)
	if err != nil {
		return probeFailed("userstat", err)
	}
	if !isTrue(v) {
		return Capability{Unavailable, "Enable userstat (SET GLOBAL userstat=1, or set it in the server config) to get INDEX_STATISTICS."}
	}
	return Capability{State: Available}
}

func detectSys(ctx context.Context, q Querier, info ServerInfo, perf Capability) Capability {
	if info.Flavor == FlavorMariaDB && !info.Version.AtLeast(10, 6, 0) {
		return Capability{Unavailable, "The sys schema needs MariaDB 10.6 or later."}
	}
	v, err := q.Scalar(ctx, qSys)
	if err != nil {
		return probeFailed("the sys schema", err)
	}
	if v.String == "" || v.String == "0" {
		return Capability{Unavailable, "Install the sys schema to get its diagnostic views."}
	}
	if info.Flavor == FlavorMariaDB && perf.State != Available {
		return Capability{Degraded, "The sys schema is present but needs performance_schema; enable it to get working sys views."}
	}
	return Capability{State: Available}
}

// replicationStatement chooses the status statement from flavor and version:
// MariaDB accepts SHOW REPLICA STATUS from 10.5.1, MySQL 8.4 rejects SHOW
// SLAVE STATUS (FEATURES.md V4, V5). The MySQL release that introduced the
// REPLICA syntax is UNVERIFIED, so MySQL before 8.4 keeps SHOW SLAVE STATUS.
// Callers should still fall back to the other form on a syntax error.
func replicationStatement(info ServerInfo) (Capability, string) {
	replica := false
	switch info.Flavor {
	case FlavorMariaDB:
		replica = info.Version.AtLeast(10, 5, 1)
	case FlavorMySQL:
		replica = info.Version.AtLeast(8, 4, 0)
	default:
		return Capability{Unavailable, "Unknown server flavor; cannot choose a replication status statement."}, ""
	}
	if replica {
		return Capability{State: Available}, "SHOW REPLICA STATUS"
	}
	return Capability{State: Available}, "SHOW SLAVE STATUS"
}

// Text renders the report for terminals.
func (r *Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "flavor:             %s\n", r.Flavor)
	fmt.Fprintf(&b, "version:            %s (%s)\n", r.Version, r.Raw)
	line := func(name string, c Capability) {
		fmt.Fprintf(&b, "%-19s %s\n", name+":", c.State)
		if c.Hint != "" {
			fmt.Fprintf(&b, "%-19s   hint: %s\n", "", c.Hint)
		}
	}
	line("performance_schema", r.PerformanceSchema)
	line("userstat", r.Userstat)
	line("sys schema", r.Sys)
	line("replication", r.Replication)
	if r.ReplicationStatement != "" {
		fmt.Fprintf(&b, "%-19s   statement: %s\n", "", r.ReplicationStatement)
	}
	return b.String()
}
