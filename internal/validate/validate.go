// Package validate checks an emitted dataset directory. It re-reads the files
// and trusts nothing about how they were produced, so it can gate a release
// even if the build code has a bug.
package validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rubichandrap/territory-of-indonesia/internal/emit"
	"github.com/rubichandrap/territory-of-indonesia/internal/geojson"
	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
)

// Severity of a finding. Errors fail the build.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Finding is one problem.
type Finding struct {
	Severity Severity `json:"severity"`
	Check    string   `json:"check"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
}

// Baseline holds the counts of a previous accepted build.
type Baseline struct {
	Counts map[string]int `json:"counts"`
}

// LoadBaseline reads a baseline file; a manifest.json works as one.
func LoadBaseline(path string) (*Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bl Baseline
	if err := json.Unmarshal(b, &bl); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(bl.Counts) == 0 {
		return nil, fmt.Errorf("%s: no counts", path)
	}
	return &bl, nil
}

// Options configure Run.
type Options struct {
	Dir string
	// Baseline, when set, makes a drop in the number of regions at any level
	// by more than MaxDrop (a fraction, 0.02 = 2%) an error.
	Baseline *Baseline
	MaxDrop  float64
}

// Result is what Run found.
type Result struct {
	Findings []Finding
	Counts   map[string]int // regions per level, from the index
	Files    map[string]int // files per tier
}

// Errors counts the findings with severity Error.
func (r *Result) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == Error {
			n++
		}
	}
	return n
}

// Bounds Indonesia fits in, with a margin.
const (
	minLon, maxLon = 94.0, 142.0
	minLat, maxLat = -12.0, 7.0
)

// maxSkippedLevels mirrors hierarchy.maxSkippedLevels: the emitted dataset
// attaches a region to a parent at most one level above the one directly above
// it. A village filed under a province skips two and is invalid.
const maxSkippedLevels = 1

type validator struct {
	opt Options
	res *Result
	mu  sync.Mutex
}

func (v *validator) add(sev Severity, check, path, format string, args ...any) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.res.Findings = append(v.res.Findings, Finding{Severity: sev, Check: check, Path: path, Message: fmt.Sprintf(format, args...)})
}

func (v *validator) errorf(check, path, format string, args ...any) {
	v.add(Error, check, path, format, args...)
}

// fileEntries is the content of one index file.
type fileEntries struct {
	level   model.Level
	parent  int64
	entries []emit.IndexEntry
}

// Run validates opt.Dir. The returned error is only for failures to read the
// directory at all; problems with the data are Findings.
func Run(opt Options) (*Result, error) {
	if st, err := os.Stat(opt.Dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("validate: %s is not a directory", opt.Dir)
	}
	v := &validator{opt: opt, res: &Result{Counts: map[string]int{}, Files: map[string]int{}}}

	m := v.checkManifest()
	files := v.loadIndex()
	v.checkIndex(files)

	for _, tier := range []string{emit.TierSimplified, emit.TierFull} {
		v.checkTier(tier, files)
	}

	if m != nil {
		for _, l := range model.Levels() {
			if got, want := v.res.Counts[l.Plural()], m.Counts[l.Plural()]; got != want {
				v.errorf("manifest-counts", "manifest.json", "%s: manifest says %d, index has %d", l.Plural(), want, got)
			}
		}
		for tier, info := range m.Tiers {
			if got := v.res.Files[tier]; got != info.Files {
				v.errorf("manifest-files", "manifest.json", "tier %s: manifest says %d files, found %d", tier, info.Files, got)
			}
		}
	}
	v.checkBaseline()

	sort.SliceStable(v.res.Findings, func(i, j int) bool {
		a, b := v.res.Findings[i], v.res.Findings[j]
		if a.Severity != b.Severity {
			return a.Severity == Error
		}
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		return a.Path < b.Path
	})
	return v.res, nil
}

func (v *validator) checkManifest() *emit.Manifest {
	path := filepath.Join(v.opt.Dir, "manifest.json")
	b, err := os.ReadFile(path)
	if err != nil {
		v.errorf("manifest", "manifest.json", "cannot read: %v", err)
		return nil
	}
	var m emit.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		v.errorf("manifest", "manifest.json", "invalid JSON: %v", err)
		return nil
	}
	if m.SchemaVersion != emit.SchemaVersion {
		v.errorf("manifest", "manifest.json", "schema_version %d, expected %d", m.SchemaVersion, emit.SchemaVersion)
	}
	if len(m.Source.Extracts) == 0 || m.Source.Timestamp == "" {
		v.add(Warning, "manifest", "manifest.json", "source extracts or osm_timestamp are missing, the build is not traceable")
	}
	return &m
}

// parseIndexPath says what an index file must contain, from its path.
func parseIndexPath(rel string) (model.Level, int64, bool) {
	rel = filepath.ToSlash(rel)
	if rel == "provinces.json" {
		return model.Province, 0, true
	}
	dir, file, ok := strings.Cut(rel, "/")
	if !ok || !strings.HasSuffix(file, ".json") {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSuffix(file, ".json"), 10, 64)
	if err != nil || id <= 0 {
		return 0, 0, false
	}
	for _, l := range []model.Level{model.Regency, model.District, model.Village} {
		if l.Plural() == dir {
			return l, id, true
		}
	}
	return 0, 0, false
}

func (v *validator) loadIndex() map[string]*fileEntries {
	root := filepath.Join(v.opt.Dir, emit.TierIndex)
	files := map[string]*fileEntries{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			v.errorf("index", emit.TierIndex, "cannot read index: %v", err)
			return fs.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		v.res.Files[emit.TierIndex]++
		level, parent, ok := parseIndexPath(rel)
		if !ok {
			v.errorf("index-path", "index/"+rel, "unexpected file in the index tree")
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			v.errorf("index", "index/"+rel, "%v", err)
			return nil
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		var entries []emit.IndexEntry
		if err := dec.Decode(&entries); err != nil {
			v.errorf("index", "index/"+rel, "invalid JSON: %v", err)
			return nil
		}
		if len(entries) == 0 {
			v.errorf("index", "index/"+rel, "file has no entries")
		}
		files[filepath.ToSlash(rel)] = &fileEntries{level: level, parent: parent, entries: entries}
		return nil
	})
	return files
}

func (v *validator) checkIndex(files map[string]*fileEntries) {
	byID := map[int64]*emit.IndexEntry{}
	paths := sortedKeys(files)

	for _, rel := range paths {
		fe := files[rel]
		path := "index/" + rel
		for i := range fe.entries {
			e := &fe.entries[i]
			v.res.Counts[fe.level.Plural()]++
			if e.OSMID <= 0 {
				v.errorf("entry", path, "entry %d has osm_id %d", i, e.OSMID)
				continue
			}
			if prev, dup := byID[e.OSMID]; dup {
				v.errorf("duplicate-id", path, "osm_id %d (%s) is also in another file (%s)", e.OSMID, e.Name, prev.Name)
			}
			byID[e.OSMID] = e
			if e.Level != fe.level.String() || e.AdminLevel != int(fe.level) {
				v.errorf("entry", path, "%d: level %q/%d does not belong in this file", e.OSMID, e.Level, e.AdminLevel)
			}
			if e.ParentID != fe.parent {
				v.errorf("entry", path, "%d: parent_id %d, file is for %d", e.OSMID, e.ParentID, fe.parent)
			}
			if strings.TrimSpace(e.Name) == "" {
				v.add(Warning, "entry-name", path, "%d has no name", e.OSMID)
			}
			b := e.BBox
			if !(b[0] < b[2] && b[1] < b[3]) || b[0] < minLon || b[2] > maxLon || b[1] < minLat || b[3] > maxLat {
				v.errorf("entry-bbox", path, "%d (%s): bbox %v is empty or outside Indonesia", e.OSMID, e.Name, b)
			}
			if c := e.Center; c[0] < b[0] || c[0] > b[2] || c[1] < b[1] || c[1] > b[3] {
				v.errorf("entry-center", path, "%d (%s): center %v is outside its bbox", e.OSMID, e.Name, c)
			}
		}
	}

	// Referential integrity and children counts.
	for _, rel := range paths {
		fe := files[rel]
		path := "index/" + rel
		if fe.parent != 0 {
			p, ok := byID[fe.parent]
			if !ok {
				v.errorf("parent-missing", path, "parent %d is not in the index", fe.parent)
			} else if gap := int(fe.level) - p.AdminLevel; gap <= 0 {
				v.errorf("parent-level", path, "parent %d (%s) is level %d, not above level %d", p.OSMID, p.Name, p.AdminLevel, int(fe.level))
			} else if gap > 1+maxSkippedLevels {
				v.errorf("parent-level", path, "parent %d (%s) is level %d: %d levels skipped between it and level %d (at most %d allowed)",
					p.OSMID, p.Name, p.AdminLevel, gap-1, int(fe.level), maxSkippedLevels)
			} else if got := p.Counts[fe.level.Plural()]; got != len(fe.entries) {
				v.errorf("children-count", "index/"+rel, "parent %d says %d %s, file has %d", p.OSMID, got, fe.level.Plural(), len(fe.entries))
			}
		}
		for i := range fe.entries {
			e := &fe.entries[i]
			if e.ParentID == 0 {
				if len(e.Ancestors) != 0 {
					v.errorf("ancestors", path, "%d has ancestors but no parent", e.OSMID)
				}
				continue
			}
			p := byID[e.ParentID]
			if p == nil {
				continue // reported above
			}
			want := append(append([]int64(nil), p.Ancestors...), p.OSMID)
			if !equalIDs(e.Ancestors, want) {
				v.errorf("ancestors", path, "%d (%s): ancestors %v, expected %v", e.OSMID, e.Name, e.Ancestors, want)
			}
		}
	}
	// Every region that claims children must have the file.
	for _, e := range byID {
		for plural, n := range e.Counts {
			if n > 0 {
				if _, ok := files[plural+"/"+strconv.FormatInt(e.OSMID, 10)+".json"]; !ok {
					v.errorf("children-file", "index/"+plural, "%d (%s) claims %d %s but has no file", e.OSMID, e.Name, n, plural)
				}
			}
		}
	}
}

func (v *validator) checkBaseline() {
	if v.opt.Baseline == nil {
		return
	}
	for plural, was := range v.opt.Baseline.Counts {
		now := v.res.Counts[plural]
		if was > 0 && float64(now) < float64(was)*(1-v.opt.MaxDrop) {
			v.errorf("regression", "", "%s dropped from %d to %d (more than %.1f%%)", plural, was, now, v.opt.MaxDrop*100)
		} else if now < was {
			v.add(Warning, "regression", "", "%s dropped from %d to %d", plural, was, now)
		}
	}
}

func (v *validator) checkTier(tier string, index map[string]*fileEntries) {
	root := filepath.Join(v.opt.Dir, tier)
	want := map[string]*fileEntries{}
	for rel, fe := range index {
		want[strings.TrimSuffix(rel, ".json")+".geojson"] = fe
	}

	var found []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			v.errorf("tier", tier, "cannot read tier: %v", err)
			return fs.SkipAll
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(found)
	v.res.Files[tier] = len(found)

	have := map[string]bool{}
	for _, rel := range found {
		have[rel] = true
		if _, ok := want[rel]; !ok {
			v.errorf("tier-extra-file", tier+"/"+rel, "no matching index file")
		}
	}
	for rel := range want {
		if !have[rel] {
			v.errorf("tier-missing-file", tier+"/"+rel, "listed in the index but not written")
		}
	}

	jobs := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range jobs {
				v.checkGeoFile(tier, rel, want[rel])
			}
		}()
	}
	for _, rel := range found {
		if want[rel] != nil {
			jobs <- rel
		}
	}
	close(jobs)
	wg.Wait()
}

func (v *validator) checkGeoFile(tier, rel string, fe *fileEntries) {
	path := tier + "/" + rel
	b, err := os.ReadFile(filepath.Join(v.opt.Dir, tier, filepath.FromSlash(rel)))
	if err != nil {
		v.errorf("geojson", path, "%v", err)
		return
	}
	var fc struct {
		Type     string            `json:"type"`
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(b, &fc); err != nil || fc.Type != "FeatureCollection" {
		v.errorf("geojson", path, "not a FeatureCollection (%v)", err)
		return
	}
	if len(fc.Features) != len(fe.entries) {
		v.errorf("geojson-count", path, "%d features, index has %d entries", len(fc.Features), len(fe.entries))
		return
	}
	for i, raw := range fc.Features {
		want := fe.entries[i]
		f, err := geojson.ParseFeature(raw)
		if err != nil {
			v.errorf("geojson", path, "feature %d: %v", i, err)
			continue
		}
		if id, _ := f.ID.(float64); int64(id) != want.OSMID {
			v.errorf("geojson-id", path, "feature %d has id %v, index says %d", i, f.ID, want.OSMID)
		}
		if n, _ := f.Properties["osm_id"].(float64); int64(n) != want.OSMID {
			v.errorf("geojson-id", path, "feature %d: properties.osm_id %v, index says %d", i, f.Properties["osm_id"], want.OSMID)
		}
		v.checkFeatureProperties(path, i, f, &want)
		if f.Geometry == nil {
			v.errorf("geometry", path, "%d (%s): geometry is %q", want.OSMID, want.Name, f.GeometryType)
			continue
		}
		if msg := geom.Validate(f.Geometry); msg != "" {
			v.errorf("geometry", path, "%d (%s): %s", want.OSMID, want.Name, msg)
			continue
		}
		if bd := f.Geometry.Bound(); math.IsNaN(bd.Min[0]) || bd.Min[0] < minLon || bd.Max[0] > maxLon || bd.Min[1] < minLat || bd.Max[1] > maxLat {
			v.errorf("geometry-bounds", path, "%d (%s): bound %v is outside Indonesia", want.OSMID, want.Name, bd)
		}
	}
}

// checkFeatureProperties makes sure a geometry feature carries the same
// identity and hierarchy properties as the index entry it accompanies. A
// client that fetches the index and the geometry separately must see one
// consistent dataset; checking only the id (as above) would let level,
// admin_level, parent_id or ancestors drift apart between the two tiers.
func (v *validator) checkFeatureProperties(path string, i int, f *geojson.Feature, want *emit.IndexEntry) {
	b, err := json.Marshal(f.Properties)
	if err != nil {
		v.errorf("feature-properties", path, "feature %d: cannot read properties: %v", i, err)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var props emit.Properties
	if err := dec.Decode(&props); err != nil {
		v.errorf("feature-properties", path, "feature %d: bad properties: %v", i, err)
		return
	}
	if !reflect.DeepEqual(props, want.Properties) {
		v.errorf("feature-properties", path, "%d (%s): feature properties %+v do not match index entry %+v",
			want.OSMID, want.Name, props, want.Properties)
	}
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
