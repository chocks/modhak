package advisor

import "sort"

// DuplicateEvidence describes indexes with identical definitions.
type DuplicateEvidence struct {
	// Kept is the index to keep; Duplicate is the one suggested for removal.
	Kept      string        `json:"kept"`
	Duplicate string        `json:"duplicate"`
	Columns   []IndexColumn `json:"columns"`
	// Note explains cases with no suggested SQL (duplicate unique indexes).
	Note string `json:"note,omitempty"`
}

func (DuplicateEvidence) evidenceKind() string { return KindDuplicateIndex }

// RedundantEvidence describes an index that is a left prefix of another.
type RedundantEvidence struct {
	Redundant  string        `json:"redundant"`
	CoveredBy  string        `json:"covered_by"`
	Columns    []IndexColumn `json:"columns"`
	CoveringBy []IndexColumn `json:"covering_columns"`
}

func (RedundantEvidence) evidenceKind() string { return KindRedundantIndex }

// comparable reports whether an index takes part in duplicate/redundant
// comparison: ordinary BTREE indexes with plain column parts. FULLTEXT,
// SPATIAL, HASH and functional (expression) indexes are skipped because
// prefix reasoning does not hold for them.
func comparable(ix Index) bool {
	if ix.Type != "" && ix.Type != "BTREE" {
		return false
	}
	if len(ix.Columns) == 0 {
		return false
	}
	for _, c := range ix.Columns {
		if c.Expression != "" || c.Name == "" {
			return false
		}
	}
	return true
}

func sameColumn(a, b IndexColumn) bool {
	return a.Name == b.Name && a.SubPart == b.SubPart && a.Desc == b.Desc
}

func sameColumns(a, b []IndexColumn) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameColumn(a[i], b[i]) {
			return false
		}
	}
	return true
}

// isPrefix reports whether a is a strict left prefix of b.
func isPrefix(a, b []IndexColumn) bool {
	if len(a) >= len(b) {
		return false
	}
	for i := range a {
		if !sameColumn(a[i], b[i]) {
			return false
		}
	}
	return true
}

func byTable(indexes []Index) map[[2]string][]Index {
	m := map[[2]string][]Index{}
	for _, ix := range indexes {
		if !comparable(ix) {
			continue
		}
		k := [2]string{ix.Schema, ix.Table}
		m[k] = append(m[k], ix)
	}
	return m
}

// keepRank orders indexes with identical columns by how much we want to keep
// them: primary, then unique, then the rest; name breaks ties.
func keepBefore(a, b Index) bool {
	ra, rb := keepRank(a), keepRank(b)
	if ra != rb {
		return ra < rb
	}
	return a.Name < b.Name
}

func keepRank(ix Index) int {
	switch {
	case ix.Primary():
		return 0
	case ix.Unique:
		return 1
	default:
		return 2
	}
}

// DuplicateIndexes finds indexes on the same table with identical column
// definitions. Only non-unique, non-primary indexes get suggested SQL; a
// duplicate unique index is reported without SQL because dropping a
// constraint is a schema decision.
func DuplicateIndexes(in Input) []Finding {
	var out []Finding
	for _, ixs := range byTable(in.Indexes) {
		sort.Slice(ixs, func(i, j int) bool { return keepBefore(ixs[i], ixs[j]) })
		kept := map[int]bool{}
		for i := range ixs {
			if kept[i] {
				continue
			}
			for j := i + 1; j < len(ixs); j++ {
				if kept[j] || !sameColumns(ixs[i].Columns, ixs[j].Columns) {
					continue
				}
				kept[j] = true
				// An ignored index is not used by the optimizer, so a
				// visible twin is not redundant with respect to it.
				if ixs[i].Ignored != ixs[j].Ignored {
					continue
				}
				dup := ixs[j]
				t := Target{Name: in.Target, Schema: dup.Schema, Table: dup.Table, Index: dup.Name}
				ev := DuplicateEvidence{Kept: ixs[i].Name, Duplicate: dup.Name, Columns: dup.Columns}
				f := Finding{Kind: KindDuplicateIndex, Severity: SeverityWarning, Target: t, Evidence: ev}
				if dup.Unique || dup.Primary() {
					ev.Note = "duplicate unique constraint; review manually before dropping"
					f.Evidence = ev
					f.Severity = SeverityInfo
				} else {
					f.SuggestedSQL = []string{dropIndexSQL(dup.Schema, dup.Table, dup.Name)}
				}
				f.ID = findingID(f.Kind, t)
				out = append(out, f)
			}
		}
	}
	return out
}

// RedundantIndexes finds non-unique indexes whose columns are a strict left
// prefix of another visible index on the same table. Foreign keys may need
// the shorter index; the server refuses such a drop with an error.
func RedundantIndexes(in Input) []Finding {
	var out []Finding
	for _, ixs := range byTable(in.Indexes) {
		for _, a := range ixs {
			if a.Unique || a.Primary() || a.Ignored {
				continue
			}
			var cover *Index
			for k := range ixs {
				b := ixs[k]
				if b.Name == a.Name || b.Ignored || !isPrefix(a.Columns, b.Columns) {
					continue
				}
				if cover == nil || keepBefore(b, *cover) {
					cp := b
					cover = &cp
				}
			}
			if cover == nil {
				continue
			}
			t := Target{Name: in.Target, Schema: a.Schema, Table: a.Table, Index: a.Name}
			out = append(out, Finding{
				ID: findingID(KindRedundantIndex, t), Kind: KindRedundantIndex, Severity: SeverityWarning, Target: t,
				Evidence:     RedundantEvidence{Redundant: a.Name, CoveredBy: cover.Name, Columns: a.Columns, CoveringBy: cover.Columns},
				SuggestedSQL: []string{dropIndexSQL(a.Schema, a.Table, a.Name)},
			})
		}
	}
	return out
}
