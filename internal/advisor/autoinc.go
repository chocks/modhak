package advisor

import (
	"strings"
	"time"
)

// AutoIncEvidence describes how much of an AUTO_INCREMENT column's range is
// used.
type AutoIncEvidence struct {
	ColumnType string `json:"column_type"`
	Unsigned   bool   `json:"unsigned"`
	Next       uint64 `json:"next"`
	Max        uint64 `json:"max"`
	// UsedFraction is (Next-1)/Max, clamped to 1.
	UsedFraction float64 `json:"used_fraction"`
	Remaining    uint64  `json:"remaining"`
	// RoughDaysToOverflow assumes the average insert rate since table
	// creation (CREATE_TIME) continues. It is a rough bound only: rates vary
	// and deleted rows still consume values. Nil when it cannot be computed
	// or the table is younger than roughMinAge (the rate would mean nothing).
	RoughDaysToOverflow *float64 `json:"rough_days_to_overflow,omitempty"`
	Note                string   `json:"note,omitempty"`
}

func (AutoIncEvidence) evidenceKind() string { return KindAutoIncHeadroom }

// roughMinAge is the youngest table for which a days-to-overflow estimate is
// shown; on a younger table the average rate says nothing.
const roughMinAge = 24 * time.Hour

// intType holds the documented ranges (verified on MariaDB 10.6/11.4 and
// MySQL 8.0/8.4; see FEATURES.md V11).
type intType struct {
	name        string
	signedMax   uint64
	unsignedMax uint64
	wider       string // next wider integer type, "" for the widest
}

var intTypes = []intType{
	{"tinyint", 127, 255, "smallint"},
	{"smallint", 32767, 65535, "mediumint"},
	{"mediumint", 8388607, 16777215, "int"},
	{"int", 2147483647, 4294967295, "bigint"},
	{"bigint", 9223372036854775807, 18446744073709551615, ""},
}

func parseIntType(columnType string) (t intType, unsigned, ok bool) {
	s := strings.ToLower(strings.TrimSpace(columnType))
	base := s
	if i := strings.IndexAny(s, "( "); i >= 0 {
		base = s[:i]
	}
	unsigned = strings.Contains(s, "unsigned")
	for _, it := range intTypes {
		if it.name == base {
			return it, unsigned, true
		}
	}
	return intType{}, false, false
}

func (t intType) max(unsigned bool) uint64 {
	if unsigned {
		return t.unsignedMax
	}
	return t.signedMax
}

// AutoIncrementHeadroom reports AUTO_INCREMENT columns that have used a large
// share of their integer range. Columns with a non-integer type are skipped.
func AutoIncrementHeadroom(in Input, cfg Config) []Finding {
	cfg = cfg.withDefaults()
	var out []Finding
	for _, a := range in.AutoIncrements {
		it, unsigned, ok := parseIntType(a.ColumnType)
		if !ok || a.Next == 0 {
			continue
		}
		max := it.max(unsigned)
		used := a.Next - 1
		frac := 1.0
		if used < max {
			frac = float64(used) / float64(max)
		}
		var sev Severity
		switch {
		case frac >= cfg.HeadroomCritical:
			sev = SeverityCritical
		case frac >= cfg.HeadroomWarn:
			sev = SeverityWarning
		default:
			continue
		}
		var remaining uint64
		if used < max {
			remaining = max - used
		}
		ev := AutoIncEvidence{ColumnType: a.ColumnType, Unsigned: unsigned, Next: a.Next, Max: max,
			UsedFraction: float64(int(frac*10000+0.5)) / 10000, Remaining: remaining}
		if !a.CreateTime.IsZero() && in.Now.Sub(a.CreateTime) >= roughMinAge && used > 0 {
			age := in.Now.Sub(a.CreateTime).Hours() / 24
			d := float64(remaining) / (float64(used) / age)
			ev.RoughDaysToOverflow = &d
		}
		t := Target{Name: in.Target, Schema: a.Schema, Table: a.Table, Column: a.Column}
		f := Finding{ID: findingID(KindAutoIncHeadroom, t), Kind: KindAutoIncHeadroom, Severity: sev, Target: t}
		if it.wider == "" {
			ev.Note = "already the widest integer type; plan a different key strategy"
		} else {
			wider := strings.ToUpper(it.wider)
			if unsigned {
				wider += " UNSIGNED"
			}
			ev.Note = "review before applying: keep the column's other attributes (such as COMMENT), and widen foreign key columns that reference it first"
			f.SuggestedSQL = []string{"ALTER TABLE " + qualified(a.Schema, a.Table) + " MODIFY " + quote(a.Column) + " " + wider + " NOT NULL AUTO_INCREMENT"}
		}
		f.Evidence = ev
		out = append(out, f)
	}
	return out
}
