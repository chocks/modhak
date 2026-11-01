package advisor

import "sort"

// Analyze runs every M1 analysis over one snapshot. The result is sorted by
// severity (critical first), then kind and location, so output is stable.
func Analyze(in Input, cfg Config) Result {
	var res Result
	res.Findings = append(res.Findings, DuplicateIndexes(in)...)
	res.Findings = append(res.Findings, RedundantIndexes(in)...)
	unused, skipped := UnusedIndexes(in, cfg)
	res.Findings = append(res.Findings, unused...)
	if skipped != nil {
		res.Skipped = append(res.Skipped, *skipped)
	}
	res.Findings = append(res.Findings, AutoIncrementHeadroom(in, cfg)...)
	sortFindings(res.Findings)
	return res
}

func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Severity.rank() != b.Severity.rank() {
			return a.Severity.rank() > b.Severity.rank()
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
}
