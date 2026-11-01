// Package advisor turns plain snapshots of a target into findings.
//
// It is pure (AGENTS.md): no HTTP, no database/sql, no store, and no clock or
// environment reads. The caller passes the current time in Input.Now.
// Output is deterministic and sorted so tests and agents can diff results.
package advisor

import "time"

// Severity ranks a finding.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

func (s Severity) rank() int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	default:
		return 1
	}
}

// Finding kinds produced in M1.
const (
	KindDuplicateIndex   = "duplicate_index"
	KindRedundantIndex   = "redundant_index"
	KindUnusedIndex      = "unused_index"
	KindAutoIncHeadroom  = "auto_increment_headroom"
	SkipUnusedIndexes    = "unused_index_analysis"
	reasonNoUsageData    = "no index usage data: enable userstat (MariaDB) or performance_schema to get unused-index analysis"
	reasonUptimeTooShort = "server uptime is below the minimum observation window"
)

// Target locates a finding. Index and Column are set when relevant.
type Target struct {
	Name   string `json:"name"`
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Index  string `json:"index,omitempty"`
	Column string `json:"column,omitempty"`
}

// Evidence is structured, kind-specific data; consumers build text from it.
type Evidence interface{ evidenceKind() string }

// Finding is one advisory result.
type Finding struct {
	// ID is stable for the same kind and target, for dedupe and links.
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Severity Severity `json:"severity"`
	Target   Target   `json:"target"`
	Evidence Evidence `json:"evidence"`
	// SuggestedSQL is text only; modhak never executes it (ADR 0007).
	SuggestedSQL []string `json:"suggested_sql,omitempty"`
}

// Skipped records an analysis that could not run, with the reason, so
// consumers can say "enable X to get Y" instead of showing nothing.
type Skipped struct {
	Analysis string `json:"analysis"`
	Reason   string `json:"reason"`
}

// Result is what Analyze returns.
type Result struct {
	Findings []Finding `json:"findings"`
	Skipped  []Skipped `json:"skipped,omitempty"`
}

// IndexColumn is one part of an index.
type IndexColumn struct {
	Name string
	// SubPart is the prefix length, 0 for the full column.
	SubPart int
	// Desc is true for a descending part (MySQL 8.0 COLLATION = 'D').
	Desc bool
	// Expression is set for functional parts (MySQL 8.0); such indexes are
	// never compared.
	Expression string
}

// Index is one index of a table, columns in order.
type Index struct {
	Schema  string
	Table   string
	Name    string
	Unique  bool
	Type    string // BTREE, HASH, FULLTEXT, SPATIAL, ...
	Columns []IndexColumn
	// Ignored is true for MariaDB IGNORED and MySQL INVISIBLE indexes: the
	// optimizer does not use them.
	Ignored bool
}

// Primary reports whether this is the primary key.
func (ix Index) Primary() bool { return ix.Name == "PRIMARY" }

// UsageSource says where read counters came from; semantics differ.
type UsageSource string

const (
	SourceUserstat   UsageSource = "userstat"
	SourcePerfSchema UsageSource = "performance_schema"
)

// IndexUsage is the read counter for one index.
type IndexUsage struct {
	Schema string
	Table  string
	Index  string
	Reads  uint64
}

// Usage is index read counters plus how long they have been counting.
type Usage struct {
	Source  UsageSource
	Uptime  time.Duration
	Entries []IndexUsage
}

// AutoIncrement is an AUTO_INCREMENT column and its next value.
type AutoIncrement struct {
	Schema string
	Table  string
	Column string
	// ColumnType is information_schema.columns.COLUMN_TYPE, e.g.
	// "int(10) unsigned" (MariaDB) or "int unsigned" (MySQL).
	ColumnType string
	// Next is information_schema.tables.AUTO_INCREMENT, the next value.
	Next uint64
	// CreateTime is the table's CREATE_TIME; zero if unknown.
	CreateTime time.Time
}

// Input is a snapshot of one target.
type Input struct {
	Target         string
	Now            time.Time
	Indexes        []Index
	Usage          *Usage // nil when no usage source is available
	AutoIncrements []AutoIncrement
}

// Config tunes thresholds. Zero values take the defaults below.
type Config struct {
	// MinUptime is the minimum observation window for unused-index verdicts
	// (default 7 days; ARCHITECTURE.md open question 5).
	MinUptime time.Duration
	// HeadroomWarn and HeadroomCritical are fractions of the type's range
	// already used (defaults 0.6 and 0.85).
	HeadroomWarn     float64
	HeadroomCritical float64
}

// DefaultConfig returns the defaults.
func DefaultConfig() Config {
	return Config{MinUptime: 7 * 24 * time.Hour, HeadroomWarn: 0.6, HeadroomCritical: 0.85}
}

func (c Config) withDefaults() Config {
	d := DefaultConfig()
	if c.MinUptime == 0 {
		c.MinUptime = d.MinUptime
	}
	if c.HeadroomWarn == 0 {
		c.HeadroomWarn = d.HeadroomWarn
	}
	if c.HeadroomCritical == 0 {
		c.HeadroomCritical = d.HeadroomCritical
	}
	return c
}
