// Package report summarises a build: how many boundaries OpenStreetMap has per
// level, how many could be turned into polygons, how many ended up in the
// hierarchy, and what looked wrong on the way.
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/rubichandrap/territory-of-indonesia/internal/emit"
	"github.com/rubichandrap/territory-of-indonesia/internal/hierarchy"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
	"github.com/rubichandrap/territory-of-indonesia/internal/osmium"
)

// maxSamples bounds the example lists in the report.
const maxSamples = 25

// Input is everything the report is computed from.
type Input struct {
	Source         emit.Source
	Skipped        map[string]int    // features osmium exported that the reader dropped, by reason
	Expected       []osmium.Relation // relations in the extract; nil when unknown
	Assembled      []*model.Region   // regions read from the export, before the hierarchy step
	Tree           *hierarchy.Tree
	ExportWarnings int
}

// Level is one row of the per-level table.
type Level struct {
	Level          string `json:"level"`
	AdminLevel     int    `json:"admin_level"`
	InOSM          int    `json:"relations_in_extract"`
	Assembled      int    `json:"polygons_assembled"`
	Kept           int    `json:"in_dataset"`
	Outside        int    `json:"outside_indonesia"`
	Orphan         int    `json:"orphans"`
	Duplicate      int    `json:"duplicates"`
	Ambiguous      int    `json:"ambiguous_parent"`
	SkippedLevel   int    `json:"attached_above_missing_level"`
	WithCode       int    `json:"with_kemendagri_code"`
	CodeChecked    int    `json:"code_checked"`
	CodeConsistent int    `json:"code_consistent"`
}

// Sample is one example in an issue group.
type Sample struct {
	OSMID  int64  `json:"osm_id"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

// IssueGroup collects the issues of one kind and level.
type IssueGroup struct {
	Kind    string   `json:"kind"`
	Level   string   `json:"level"`
	Count   int      `json:"count"`
	Samples []Sample `json:"samples"`
}

// Unassembled is a relation that exists in OSM but produced no polygon.
type Unassembled struct {
	OSMID      int64  `json:"osm_id"`
	AdminLevel int    `json:"admin_level"`
	Name       string `json:"name"`
}

// Report is report.json.
type Report struct {
	Source         emit.Source    `json:"source"`
	Levels         []Level        `json:"levels"`
	Issues         []IssueGroup   `json:"issues"`
	Unassembled    []Unassembled  `json:"unassembled_relations"`
	UnassembledN   map[string]int `json:"unassembled_relations_by_level"`
	Skipped        map[string]int `json:"skipped_features,omitempty"`
	ExportWarnings int            `json:"osmium_export_log_lines"`
}

// Build computes the report.
func Build(in Input) *Report {
	rep := &Report{
		Source:         in.Source,
		Skipped:        in.Skipped,
		ExportWarnings: in.ExportWarnings,
		UnassembledN:   map[string]int{},
	}

	expected := map[model.Level]int{}
	assembled := map[int64]bool{}
	for _, r := range in.Assembled {
		assembled[r.OSMID] = true
	}
	for _, rel := range in.Expected {
		lvl, ok := expectedLevel(rel)
		if !ok {
			continue
		}
		expected[lvl]++
		if !assembled[rel.ID] {
			rep.UnassembledN[lvl.Plural()]++
			rep.Unassembled = append(rep.Unassembled, Unassembled{OSMID: rel.ID, AdminLevel: int(lvl), Name: rel.Tags["name"]})
		}
	}
	sort.Slice(rep.Unassembled, func(i, j int) bool {
		a, b := rep.Unassembled[i], rep.Unassembled[j]
		if a.AdminLevel != b.AdminLevel {
			return a.AdminLevel < b.AdminLevel
		}
		return a.OSMID < b.OSMID
	})

	for _, st := range in.Tree.Stats {
		rep.Levels = append(rep.Levels, Level{
			Level: st.Level.String(), AdminLevel: int(st.Level),
			InOSM: expected[st.Level], Assembled: st.Input, Kept: st.Kept,
			Outside: st.Outside, Orphan: st.Orphan, Duplicate: st.Duplicate, Ambiguous: st.Ambiguous, SkippedLevel: st.SkippedLevel,
			WithCode: st.WithCode, CodeChecked: st.CodeChecked, CodeConsistent: st.CodeConsistent,
		})
	}

	groups := map[[2]string]*IssueGroup{}
	for _, is := range in.Tree.Issues {
		key := [2]string{string(is.Kind), is.Level.String()}
		g := groups[key]
		if g == nil {
			g = &IssueGroup{Kind: key[0], Level: key[1]}
			groups[key] = g
		}
		g.Count++
		if len(g.Samples) < maxSamples {
			g.Samples = append(g.Samples, Sample{OSMID: is.OSMID, Name: is.Name, Detail: is.Detail})
		}
	}
	for _, g := range groups {
		rep.Issues = append(rep.Issues, *g)
	}
	sort.Slice(rep.Issues, func(i, j int) bool {
		a, b := rep.Issues[i], rep.Issues[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Level < b.Level
	})
	return rep
}

// expectedLevel says whether a relation is a boundary the build is meant to
// publish, and at which level.
func expectedLevel(rel osmium.Relation) (model.Level, bool) {
	if rel.Tags["boundary"] != "administrative" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(rel.Tags["admin_level"]))
	if err != nil || !model.Level(n).Valid() {
		return 0, false
	}
	return model.Level(n), true
}

// WriteJSON writes the report as indented JSON, with the unassembled list
// capped so the file stays readable.
func (r *Report) WriteJSON(path string) error {
	c := *r
	if len(c.Unassembled) > 500 {
		c.Unassembled = c.Unassembled[:500]
	}
	b, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Markdown renders the report for humans (a CI job summary, a release note).
func (r *Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Build report\n\n")
	if len(r.Source.Extracts) > 0 {
		b.WriteString("Source:")
		for i, e := range r.Source.Extracts {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, " `%s`", e.File)
		}
		if r.Source.Timestamp != "" {
			fmt.Fprintf(&b, ", OSM data as of %s", r.Source.Timestamp)
		}
		b.WriteString("\n\n")
	}

	b.WriteString("| Level | In OSM | Assembled | In dataset | Outside Indonesia | Orphans | Attached above a missing level | With code | Code matches parent |\n")
	b.WriteString("|---|--:|--:|--:|--:|--:|--:|--:|--:|\n")
	for _, l := range r.Levels {
		match := "n/a"
		if l.CodeChecked > 0 {
			match = fmt.Sprintf("%d / %d (%.1f%%)", l.CodeConsistent, l.CodeChecked, 100*float64(l.CodeConsistent)/float64(l.CodeChecked))
		}
		inOSM := "n/a"
		if l.InOSM > 0 {
			inOSM = strconv.Itoa(l.InOSM)
		}
		fmt.Fprintf(&b, "| %s (%d) | %s | %d | %d | %d | %d | %d | %d | %s |\n",
			l.Level, l.AdminLevel, inOSM, l.Assembled, l.Kept, l.Outside, l.Orphan, l.SkippedLevel, l.WithCode, match)
	}

	if len(r.UnassembledN) > 0 {
		b.WriteString("\n## Relations OSM has but osmium could not turn into polygons\n\n")
		b.WriteString("These are usually relations whose ways do not close into rings (a mapping error) or whose members are outside the extract, which is the normal case for foreign boundaries at the border.\n\n")
		for _, l := range model.Levels() {
			if n := r.UnassembledN[l.Plural()]; n > 0 {
				fmt.Fprintf(&b, "- %s: %d\n", l.Plural(), n)
			}
		}
	}

	if len(r.Issues) > 0 {
		b.WriteString("\n## Issues\n\n| Kind | Level | Count | Examples |\n|---|---|--:|---|\n")
		for _, g := range r.Issues {
			var ex []string
			for i, s := range g.Samples {
				if i == 3 {
					break
				}
				ex = append(ex, fmt.Sprintf("%s (%d)", s.Name, s.OSMID))
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", g.Kind, g.Level, g.Count, strings.Join(ex, ", "))
		}
	}
	if len(r.Skipped) > 0 {
		b.WriteString("\n## Exported by osmium but not used\n\n")
		keys := make([]string, 0, len(r.Skipped))
		for k := range r.Skipped {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s: %d\n", k, r.Skipped[k])
		}
	}
	return b.String()
}
