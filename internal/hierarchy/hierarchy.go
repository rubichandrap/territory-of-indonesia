// Package hierarchy decides which region sits inside which.
//
// OpenStreetMap does not store the administrative tree in a usable way, so
// the parent of a region is found geometrically: the region's interior point
// is tested against the polygons one level up, then two levels up and so on.
// The level-skipping fallback matters because OSM is far from complete below
// the regency level: a village whose kecamatan is not mapped is attached to
// its kabupaten instead of being dropped. At most one level may be skipped. A
// village that only a province contains is not a village boundary the
// hierarchy can place (in practice: a mis-tagged beach in the sea), so it is
// reported as an orphan and left out.
package hierarchy

import (
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/paulmach/orb"

	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
)

// IssueKind classifies something the build noticed about a region.
type IssueKind string

const (
	// Outside: no Indonesian province contains the region. Foreign
	// boundaries that happen to be in the extract end up here.
	Outside IssueKind = "outside_indonesia"
	// Duplicate: the same relation id was seen twice.
	Duplicate IssueKind = "duplicate_id"
	// Ambiguous: more than one region of the level above contains the
	// region's interior point (overlapping or enclosing parents).
	Ambiguous IssueKind = "ambiguous_parent"
	// SkippedLevel: the parent is two levels up, because the level in
	// between is not mapped around this region.
	SkippedLevel IssueKind = "skipped_level"
	// Orphan: inside an Indonesian province, but no region within one
	// skipped level contains it. It is left out of the dataset.
	Orphan IssueKind = "orphan"
	// CodeMismatch: the region's Kemendagri code does not start with its
	// parent's code, which points at a wrong parent or a wrong code.
	CodeMismatch IssueKind = "code_mismatch"
)

// Issue is one observation about one region.
type Issue struct {
	Kind   IssueKind
	Level  model.Level
	OSMID  int64
	Name   string
	Detail string
}

// LevelStats summarises one level.
type LevelStats struct {
	Level          model.Level
	Input          int // regions offered to Build at this level
	Kept           int // regions in the tree
	Outside        int
	Orphan         int // inside Indonesia but not placeable in the hierarchy; left out
	Duplicate      int
	Ambiguous      int
	SkippedLevel   int // kept, but attached more than one level up
	WithCode       int // kept regions that have a Kemendagri code
	CodeChecked    int // kept regions where both own and parent code exist
	CodeConsistent int // ... and the parent code is a prefix
}

// Tree is the resolved hierarchy.
type Tree struct {
	// Regions holds every kept region ordered by level, name, id.
	Regions []*model.Region
	ByID    map[int64]*model.Region
	// Children maps a parent id to its direct children, ordered by name.
	// Key 0 holds the provinces.
	Children map[int64][]*model.Region
	Issues   []Issue
	Stats    []LevelStats // one per level, top down
}

// Build resolves the hierarchy for regions. The input is not modified except
// for ParentID and Ancestors on regions that are kept.
func Build(in []*model.Region) *Tree {
	t := &Tree{
		ByID:     map[int64]*model.Region{},
		Children: map[int64][]*model.Region{},
	}
	stats := map[model.Level]*LevelStats{}
	for _, l := range model.Levels() {
		stats[l] = &LevelStats{Level: l}
	}

	// Deterministic order regardless of how the PBF was read.
	sorted := append([]*model.Region(nil), in...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].OSMID < sorted[j].OSMID })

	byLevel := map[model.Level][]*model.Region{}
	seen := map[int64]bool{}
	for _, r := range sorted {
		st, ok := stats[r.Level]
		if !ok {
			continue
		}
		st.Input++
		if seen[r.OSMID] {
			st.Duplicate++
			t.issue(Duplicate, r, "relation id appears more than once")
			continue
		}
		seen[r.OSMID] = true
		byLevel[r.Level] = append(byLevel[r.Level], r)
	}

	kept := map[model.Level][]*model.Region{}

	for _, r := range byLevel[model.Province] {
		if !strings.HasPrefix(r.ISO3166_2, "ID-") {
			stats[r.Level].Outside++
			t.issue(Outside, r, "province without an ISO3166-2 code starting with ID-")
			continue
		}
		t.keep(r, nil, stats)
		kept[model.Province] = append(kept[model.Province], r)
	}

	for _, level := range []model.Level{model.Regency, model.District, model.Village} {
		regs := byLevel[level]
		res := resolveAll(regs, kept)
		for i, r := range regs {
			p := res[i]
			if p.orphan {
				stats[level].Orphan++
				t.issue(Orphan, r, "inside a province, but no region of level "+strconv.Itoa(int(level)-1)+" or "+strconv.Itoa(int(level)-2)+" contains its interior point")
				continue
			}
			if p.parent == nil {
				stats[level].Outside++
				t.issue(Outside, r, "no Indonesian province contains its interior point")
				continue
			}
			if p.ambiguous > 1 {
				stats[level].Ambiguous++
				t.issue(Ambiguous, r, "contained in "+strconv.Itoa(p.ambiguous)+" regions of level "+strconv.Itoa(int(p.parent.Level))+", chose "+p.parent.Name+" by "+p.method)
			}
			if p.parent.Level != level-1 {
				stats[level].SkippedLevel++
				t.issue(SkippedLevel, r, "attached to a level "+strconv.Itoa(int(p.parent.Level))+" parent: "+p.parent.Name)
			}
			t.keep(r, p.parent, stats)
			kept[level] = append(kept[level], r)
		}
	}

	for _, l := range model.Levels() {
		t.Stats = append(t.Stats, *stats[l])
	}
	t.sort()
	return t
}

func (t *Tree) issue(kind IssueKind, r *model.Region, detail string) {
	t.Issues = append(t.Issues, Issue{Kind: kind, Level: r.Level, OSMID: r.OSMID, Name: r.Name, Detail: detail})
}

func (t *Tree) keep(r, parent *model.Region, stats map[model.Level]*LevelStats) {
	st := stats[r.Level]
	st.Kept++
	if r.Code != "" {
		st.WithCode++
	}
	if parent != nil {
		r.ParentID = parent.OSMID
		r.Ancestors = append(append([]int64(nil), parent.Ancestors...), parent.OSMID)
		if r.Code != "" && parent.Code != "" {
			st.CodeChecked++
			if model.CodeHasPrefix(r.Code, parent.Code) {
				st.CodeConsistent++
			} else {
				t.issue(CodeMismatch, r, "code "+r.Code+" is not under parent "+parent.Name+" ("+parent.Code+")")
			}
		}
	} else {
		r.ParentID, r.Ancestors = 0, nil
	}
	t.ByID[r.OSMID] = r
	t.Regions = append(t.Regions, r)
	t.Children[r.ParentID] = append(t.Children[r.ParentID], r)
}

func (t *Tree) sort() {
	less := func(a, b *model.Region) bool {
		if an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name); an != bn {
			return an < bn
		}
		return a.OSMID < b.OSMID
	}
	sort.Slice(t.Regions, func(i, j int) bool {
		a, b := t.Regions[i], t.Regions[j]
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return less(a, b)
	})
	for _, kids := range t.Children {
		sort.Slice(kids, func(i, j int) bool { return less(kids[i], kids[j]) })
	}
}

type resolution struct {
	orphan    bool // inside a province but not placeable
	parent    *model.Region
	ambiguous int // number of candidates at the level where the parent was found; 0 or 1 means unambiguous
	method    string
}

// resolveAll finds a parent for every region, in parallel. The result has the
// same order as regs.
func resolveAll(regs []*model.Region, kept map[model.Level][]*model.Region) []resolution {
	out := make([]resolution, len(regs))
	workers := runtime.NumCPU()
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				out[i] = resolveOne(regs[i], kept)
			}
		}()
	}
	for i := range regs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

// maxSkippedLevels is how many missing levels a region may jump over.
const maxSkippedLevels = 1

func resolveOne(r *model.Region, kept map[model.Level][]*model.Region) resolution {
	lowest := r.Level - 1 - maxSkippedLevels
	if lowest < model.Province {
		lowest = model.Province
	}
	for pl := r.Level - 1; pl >= lowest; pl-- {
		cands := containing(kept[pl], r.Center)
		switch len(cands) {
		case 0:
			continue
		case 1:
			return resolution{parent: cands[0], ambiguous: 1, method: "containment"}
		default:
			p, method := pick(r, cands)
			return resolution{parent: p, ambiguous: len(cands), method: method}
		}
	}
	// Not placeable. If the search stopped above the provinces, check whether
	// the region is at least in Indonesia.
	if lowest > model.Province && len(containing(kept[model.Province], r.Center)) > 0 {
		return resolution{orphan: true}
	}
	return resolution{}
}

func containing(cands []*model.Region, pt orb.Point) []*model.Region {
	var out []*model.Region
	for _, c := range cands {
		if c.Bound.Contains(pt) && geom.Contains(c.Geometry, pt) {
			out = append(out, c)
		}
	}
	return out
}

// pick chooses between several parents that all contain the region. A parent
// whose code prefixes the region's code wins when exactly one does; otherwise
// the smallest one wins, which is right for an enclave such as a kota inside
// the polygon of the surrounding kabupaten.
func pick(r *model.Region, cands []*model.Region) (*model.Region, string) {
	if r.Code != "" {
		var m []*model.Region
		for _, c := range cands {
			if model.CodeHasPrefix(r.Code, c.Code) {
				m = append(m, c)
			}
		}
		if len(m) == 1 {
			return m[0], "code"
		}
		if len(m) > 1 {
			cands = m
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Area != cands[j].Area {
			return cands[i].Area < cands[j].Area
		}
		return cands[i].OSMID < cands[j].OSMID
	})
	return cands[0], "smallest area"
}
