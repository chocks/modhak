package scan

import (
	"fmt"
	"strings"

	"github.com/chocks/modhak/internal/advisor"
)

// Text renders results for a terminal. Wording is built here from the
// structured evidence; the advisor never produces prose.
func Text(results []TargetResult) string {
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "== %s (%s)", r.Target, r.Addr)
		if r.Capabilities != nil {
			fmt.Fprintf(&b, "  %s %s", r.Capabilities.Flavor, r.Capabilities.Version)
		}
		b.WriteByte('\n')
		if r.Failed() {
			fmt.Fprintf(&b, "error: %s\n", r.Error)
			continue
		}
		if len(r.Findings) == 0 {
			b.WriteString("no findings\n")
		} else {
			fmt.Fprintf(&b, "%d %s\n", len(r.Findings), plural(len(r.Findings), "finding", "findings"))
		}
		for _, f := range r.Findings {
			b.WriteByte('\n')
			fmt.Fprintf(&b, "[%s] %s  %s\n", f.Severity, f.Kind, location(f.Target))
			for _, line := range describe(f) {
				fmt.Fprintf(&b, "    %s\n", line)
			}
			for _, q := range f.SuggestedSQL {
				fmt.Fprintf(&b, "    suggested: %s;\n", q)
			}
		}
		for _, s := range r.Skipped {
			fmt.Fprintf(&b, "\nskipped %s: %s\n", s.Analysis, s.Reason)
		}
	}
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func location(t advisor.Target) string {
	s := t.Schema + "." + t.Table
	if t.Index != "" {
		s += "." + t.Index
	}
	if t.Column != "" {
		s += "." + t.Column
	}
	return s
}

func cols(cs []advisor.IndexColumn) string {
	var parts []string
	for _, c := range cs {
		p := c.Name
		if c.Expression != "" {
			p = c.Expression
		}
		if c.SubPart > 0 {
			p += fmt.Sprintf("(%d)", c.SubPart)
		}
		if c.Desc {
			p += " DESC"
		}
		parts = append(parts, p)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func describe(f advisor.Finding) []string {
	switch ev := f.Evidence.(type) {
	case advisor.DuplicateEvidence:
		lines := []string{fmt.Sprintf("%s has the same columns %s as %s", ev.Duplicate, cols(ev.Columns), ev.Kept)}
		if ev.Note != "" {
			lines = append(lines, ev.Note)
		}
		return lines
	case advisor.RedundantEvidence:
		return []string{fmt.Sprintf("%s %s is a left prefix of %s %s", ev.Redundant, cols(ev.Columns), ev.CoveredBy, cols(ev.CoveringBy))}
	case advisor.UnusedEvidence:
		return []string{
			fmt.Sprintf("0 reads in %.1f days (source: %s)", ev.ObservedDays, ev.Source),
			ev.Caveat,
		}
	case advisor.AutoIncEvidence:
		lines := []string{fmt.Sprintf("%s: next value %d of %d, %.1f%% of the range used, %d left",
			ev.ColumnType, ev.Next, ev.Max, ev.UsedFraction*100, ev.Remaining)}
		if ev.RoughDaysToOverflow != nil {
			lines = append(lines, fmt.Sprintf("rough estimate: about %.0f days left at the average rate since table creation", *ev.RoughDaysToOverflow))
		}
		if ev.Note != "" {
			lines = append(lines, ev.Note)
		}
		return lines
	}
	return nil
}
