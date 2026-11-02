package advisor

import (
	"fmt"
	"time"
)

// UnusedEvidence carries the observation window and the caveats that must
// accompany every unused-index verdict (ADR 0007).
type UnusedEvidence struct {
	Source UsageSource `json:"source"`
	Reads  uint64      `json:"reads"`
	// ObservedFor is the server uptime; counters reset on restart.
	ObservedFor  time.Duration `json:"observed_for_ns"`
	ObservedDays float64       `json:"observed_days"`
	Caveat       string        `json:"caveat"`
}

func (UnusedEvidence) evidenceKind() string { return KindUnusedIndex }

const unusedCaveat = "Observed for %s since the server started. Counters reset on restart and may reset on DDL; " +
	"queries that run rarely (monthly reports, batch jobs) or only on other servers (primary vs replica) may not appear in this window."

// UnusedIndexes reports non-unique, non-primary, visible indexes that show
// zero reads. It returns no findings, and a reason, when usage data is
// missing or the observation window is shorter than cfg.MinUptime. When
// findings are returned it can still return a note for indexes that have no
// counter row (MySQL has none for a table not opened since startup, V12):
// those are unknown, not unused.
func UnusedIndexes(in Input, cfg Config) ([]Finding, *Skipped) {
	cfg = cfg.withDefaults()
	if in.Usage == nil {
		reason := reasonNoUsageData
		if in.UsageUnavailable != "" {
			reason = in.UsageUnavailable
		}
		return nil, &Skipped{Analysis: SkipUnusedIndexes, Reason: reason}
	}
	if in.Usage.Uptime < cfg.MinUptime {
		return nil, &Skipped{Analysis: SkipUnusedIndexes,
			Reason: reasonUptimeTooShort + ": uptime " + durationDays(in.Usage.Uptime) + " days, minimum " + durationDays(cfg.MinUptime) + " days"}
	}
	reads := map[[3]string]uint64{}
	for _, e := range in.Usage.Entries {
		reads[[3]string{e.Schema, e.Table, e.Index}] += e.Reads
	}
	var out []Finding
	unknown := 0
	for _, ix := range in.Indexes {
		if ix.Unique || ix.Primary() || ix.Ignored {
			continue
		}
		n, seen := reads[[3]string{ix.Schema, ix.Table, ix.Name}]
		// An index with no counter row is unknown, not unused.
		if !seen {
			unknown++
			continue
		}
		if n != 0 {
			continue
		}
		t := Target{Name: in.Target, Schema: ix.Schema, Table: ix.Table, Index: ix.Name}
		days := in.Usage.Uptime.Hours() / 24
		out = append(out, Finding{
			ID: findingID(KindUnusedIndex, t), Kind: KindUnusedIndex, Severity: SeverityInfo, Target: t,
			Evidence: UnusedEvidence{Source: in.Usage.Source, Reads: n, ObservedFor: in.Usage.Uptime, ObservedDays: round1(days),
				Caveat: fmt.Sprintf(unusedCaveat, humanDuration(in.Usage.Uptime))},
			SuggestedSQL: []string{dropIndexSQL(ix.Schema, ix.Table, ix.Name)},
		})
	}
	var note *Skipped
	if unknown > 0 {
		note = &Skipped{Analysis: SkipUnusedIndexes, Reason: fmt.Sprintf(
			"%d %s no usage counters yet (the table has not been accessed since the server started), so no verdict is given for %s",
			unknown, pluralize(unknown, "index has", "indexes have"), pluralize(unknown, "it", "them"))}
	}
	return out, note
}

func pluralize(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
