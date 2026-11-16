// Package emit writes the v2 dataset to disk.
//
// Layout, relative to the output directory:
//
//	manifest.json
//	index/provinces.json               geometry-free entries, for dropdowns
//	index/<level>/<parent id>.json     children of one region
//	simplified/provinces.geojson       FeatureCollections, simplified geometry
//	simplified/<level>/<parent id>.geojson
//	full/...                           the same, with OSM's full geometry
//
// <level> is regencies, districts or villages. The parent id is the id of the
// nearest ancestor that exists in the dataset, which is normally the level
// directly above but can be higher where OSM lacks the level in between.
package emit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/paulmach/orb"

	"github.com/rubichandrap/territory-of-indonesia/internal/geojson"
	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
	"github.com/rubichandrap/territory-of-indonesia/internal/hierarchy"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
)

// SchemaVersion is the major version of the output format.
const SchemaVersion = 2

// Tier names, also the directory names.
const (
	TierIndex      = "index"
	TierSimplified = "simplified"
	TierFull       = "full"
)

// Properties are the attributes of a region. They are the properties of every
// GeoJSON feature and the base of every index entry.
type Properties struct {
	OSMID        int64   `json:"osm_id"`
	Level        string  `json:"level"`
	AdminLevel   int     `json:"admin_level"`
	Kind         string  `json:"kind,omitempty"`
	Name         string  `json:"name"`
	NameEN       string  `json:"name_en,omitempty"`
	OfficialName string  `json:"official_name,omitempty"`
	Code         string  `json:"code,omitempty"`
	Wikidata     string  `json:"wikidata,omitempty"`
	ParentID     int64   `json:"parent_id,omitempty"`
	Ancestors    []int64 `json:"ancestors,omitempty"`
}

// IndexEntry is Properties plus what a client needs to show a region on a map
// without loading its geometry.
type IndexEntry struct {
	Properties
	BBox   [4]float64     `json:"bbox"`
	Center [2]float64     `json:"center"`
	Counts map[string]int `json:"children,omitempty"` // child level (plural) -> number of children
}

// PropertiesOf converts a region.
func PropertiesOf(r *model.Region) Properties {
	return Properties{
		OSMID:        r.OSMID,
		Level:        r.Level.String(),
		AdminLevel:   int(r.Level),
		Kind:         r.Kind,
		Name:         r.Name,
		NameEN:       r.NameEN,
		OfficialName: r.OfficialName,
		Code:         r.Code,
		Wikidata:     r.Wikidata,
		ParentID:     r.ParentID,
		Ancestors:    r.Ancestors,
	}
}

// Options configure Write.
type Options struct {
	Dir string
	// Tolerance is the Douglas-Peucker tolerance in degrees per level for the
	// simplified tier. A missing level uses DefaultTolerance.
	Tolerance map[model.Level]float64
	// Decimals is the coordinate precision of the simplified tier.
	Decimals int
	// MinIsland overrides DefaultMinIsland.
	MinIsland map[model.Level]float64
	// Source describes where the data came from; copied into the manifest.
	Source Source
	// Tool is the version string recorded in the manifest.
	Tool string
}

// DefaultTolerance and DefaultDecimals are the simplified-tier settings the
// command line uses. A degree of latitude is about 111 km, so 0.0005 is
// roughly 55 m.
var (
	DefaultTolerance = map[model.Level]float64{
		model.Province: 0.008,
		model.Regency:  0.002,
		model.District: 0.001,
		model.Village:  0.0005,
	}
	DefaultDecimals = 5

	// DefaultMinIsland is the smallest polygon, in square degrees, the
	// simplified tier keeps for a level. A province is drawn at country scale,
	// where an island under about 5 km2 is far smaller than a pixel, and there
	// are thousands of them. Regencies, districts and villages keep every
	// polygon, and so does the full tier.
	DefaultMinIsland = map[model.Level]float64{model.Province: 0.0004}
)

// Extract is one input file of a build.
type Extract struct {
	File      string `json:"file"`
	URL       string `json:"url,omitempty"`
	SHA256    string `json:"sha256"`
	Timestamp string `json:"osm_timestamp,omitempty"` // data in this file is current as of, UTC
}

// Source is the provenance block of the manifest.
type Source struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	// Timestamp is the oldest Timestamp of the extracts: the data is at least
	// this current.
	Timestamp string    `json:"osm_timestamp,omitempty"`
	Extracts  []Extract `json:"extracts,omitempty"`
}

// TierInfo describes one directory of the output.
type TierInfo struct {
	Dir         string             `json:"dir"`
	Description string             `json:"description"`
	Decimals    int                `json:"decimals,omitempty"`
	Tolerance   map[string]float64 `json:"simplify_tolerance_degrees,omitempty"`
	Files       int                `json:"files"`
	Bytes       int64              `json:"bytes"`
}

// Manifest is manifest.json.
type Manifest struct {
	SchemaVersion int                 `json:"schema_version"`
	Source        Source              `json:"source"`
	Tool          string              `json:"tool"`
	Counts        map[string]int      `json:"counts"`
	Tiers         map[string]TierInfo `json:"tiers"`
	License       string              `json:"license"`
	Attribution   string              `json:"attribution"`
}

// Write emits the whole dataset and returns the manifest it wrote. Existing
// output of a previous run in opt.Dir is replaced; other files are left alone.
func Write(t *hierarchy.Tree, opt Options) (*Manifest, error) {
	if opt.Dir == "" {
		return nil, fmt.Errorf("emit: no output directory")
	}
	if opt.Tolerance == nil {
		opt.Tolerance = DefaultTolerance
	}
	if opt.Decimals == 0 {
		opt.Decimals = DefaultDecimals
	}
	if opt.MinIsland == nil {
		opt.MinIsland = DefaultMinIsland
	}
	for _, name := range []string{TierIndex, TierSimplified, TierFull, "manifest.json"} {
		if err := os.RemoveAll(filepath.Join(opt.Dir, name)); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
		return nil, err
	}

	groups := groupsOf(t)
	counts := childCounts(t)

	type tierStat struct{ files, bytes atomic.Int64 }
	var stats [3]tierStat // index, simplified, full

	jobs := make(chan group)
	errc := make(chan error, 1)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for g := range jobs {
				for ti, tier := range []string{TierIndex, TierSimplified, TierFull} {
					n, err := writeGroup(opt, tier, g, counts)
					if err != nil {
						select {
						case errc <- err:
						default:
						}
						return
					}
					stats[ti].files.Add(1)
					stats[ti].bytes.Add(n)
				}
			}
		}()
	}
	for _, g := range groups {
		select {
		case jobs <- g:
		case err := <-errc:
			close(jobs)
			wg.Wait()
			return nil, err
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errc:
		return nil, err
	default:
	}

	m := &Manifest{
		SchemaVersion: SchemaVersion,
		Source:        opt.Source,
		Tool:          opt.Tool,
		Counts:        map[string]int{},
		License:       "ODbL-1.0",
		Attribution:   "© OpenStreetMap contributors",
		Tiers: map[string]TierInfo{
			TierIndex: {
				Dir: TierIndex, Description: "Properties, bounding box and a label point for every region; no geometry. Use it to fill dropdowns.",
				Files: int(stats[0].files.Load()), Bytes: stats[0].bytes.Load(),
			},
			TierSimplified: {
				Dir: TierSimplified, Description: "GeoJSON with Douglas-Peucker simplified geometry, for drawing maps. Shared borders are not guaranteed to match exactly, and province outlines leave out islands smaller than about 5 km2.",
				Decimals: opt.Decimals, Tolerance: toleranceNames(opt.Tolerance),
				Files: int(stats[1].files.Load()), Bytes: stats[1].bytes.Load(),
			},
			TierFull: {
				Dir: TierFull, Description: "GeoJSON with the geometry exactly as assembled from OpenStreetMap.",
				Files: int(stats[2].files.Load()), Bytes: stats[2].bytes.Load(),
			},
		},
	}
	for _, l := range model.Levels() {
		m.Counts[l.Plural()] = 0
	}
	for _, r := range t.Regions {
		m.Counts[r.Level.Plural()]++
	}
	if err := writeJSON(filepath.Join(opt.Dir, "manifest.json"), m, true); err != nil {
		return nil, err
	}
	return m, nil
}

func toleranceNames(in map[model.Level]float64) map[string]float64 {
	out := map[string]float64{}
	for l, v := range in {
		out[l.Plural()] = v
	}
	return out
}

// group is the set of regions of one level that share a parent. It becomes one
// file in each tier.
type group struct {
	level  model.Level
	parent int64 // 0: provinces
	items  []*model.Region
}

func (g group) relPath(ext string) string {
	if g.level == model.Province {
		return "provinces" + ext
	}
	return filepath.Join(g.level.Plural(), strconv.FormatInt(g.parent, 10)+ext)
}

func groupsOf(t *hierarchy.Tree) []group {
	var out []group
	for parent, kids := range t.Children {
		byLevel := map[model.Level][]*model.Region{}
		for _, k := range kids {
			byLevel[k.Level] = append(byLevel[k.Level], k)
		}
		for l, items := range byLevel {
			out = append(out, group{level: l, parent: parent, items: items})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].level != out[j].level {
			return out[i].level < out[j].level
		}
		return out[i].parent < out[j].parent
	})
	return out
}

func childCounts(t *hierarchy.Tree) map[int64]map[string]int {
	out := map[int64]map[string]int{}
	for parent, kids := range t.Children {
		if parent == 0 {
			continue
		}
		m := map[string]int{}
		for _, k := range kids {
			m[k.Level.Plural()]++
		}
		out[parent] = m
	}
	return out
}

// writeGroup writes one file of one tier and returns its size.
func writeGroup(opt Options, tier string, g group, counts map[int64]map[string]int) (int64, error) {
	switch tier {
	case TierIndex:
		entries := make([]IndexEntry, len(g.items))
		for i, r := range g.items {
			entries[i] = IndexEntry{
				Properties: PropertiesOf(r),
				BBox:       [4]float64{r.Bound.Min[0], r.Bound.Min[1], r.Bound.Max[0], r.Bound.Max[1]},
				Center:     [2]float64{roundTo(r.Center[0], 5), roundTo(r.Center[1], 5)},
				Counts:     counts[r.OSMID],
			}
		}
		return writeFile(filepath.Join(opt.Dir, TierIndex, g.relPath(".json")), func(w *bufio.Writer) error {
			b, err := json.Marshal(entries)
			if err != nil {
				return err
			}
			_, err = w.Write(append(b, '\n'))
			return err
		})
	default:
		return writeFile(filepath.Join(opt.Dir, tier, g.relPath(".geojson")), func(w *bufio.Writer) error {
			if _, err := w.WriteString(`{"type":"FeatureCollection","features":[`); err != nil {
				return err
			}
			var buf []byte
			for i, r := range g.items {
				geometry := r.Geometry
				if tier == TierSimplified {
					geometry = simplified(r, opt)
				}
				var err error
				if buf, err = appendFeature(buf[:0], r, geometry); err != nil {
					return fmt.Errorf("region %d: %w", r.OSMID, err)
				}
				if i > 0 {
					if err := w.WriteByte(','); err != nil {
						return err
					}
				}
				if _, err := w.Write(buf); err != nil {
					return err
				}
			}
			_, err := w.WriteString("]}\n")
			return err
		})
	}
}

// simplified returns the drawing geometry of r. If simplifying or rounding
// ever yields something invalid, it falls back to progressively less
// aggressive versions rather than shipping a broken polygon.
func simplified(r *model.Region, opt Options) orb.Geometry {
	tol, ok := opt.Tolerance[r.Level]
	if !ok {
		tol = DefaultTolerance[r.Level]
	}
	src := geom.DropSmall(r.Geometry, opt.MinIsland[r.Level])
	if g := geom.Round(geom.Simplify(src, tol), opt.Decimals); geom.Validate(g) == "" {
		return g
	}
	if g := geom.Round(r.Geometry, opt.Decimals); geom.Validate(g) == "" {
		return g
	}
	return r.Geometry
}

func appendFeature(dst []byte, r *model.Region, g orb.Geometry) ([]byte, error) {
	props, err := json.Marshal(PropertiesOf(r))
	if err != nil {
		return dst, err
	}
	dst = append(dst, `{"type":"Feature","id":`...)
	dst = strconv.AppendInt(dst, r.OSMID, 10)
	dst = append(dst, `,"bbox":`...)
	dst = geojson.AppendBBox(dst, g.Bound())
	dst = append(dst, `,"properties":`...)
	dst = append(dst, props...)
	dst = append(dst, `,"geometry":`...)
	if dst, err = geojson.AppendGeometry(dst, g); err != nil {
		return dst, err
	}
	return append(dst, '}'), nil
}

func writeFile(path string, fill func(*bufio.Writer) error) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	if err := fill(w); err != nil {
		f.Close()
		return 0, err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return 0, err
	}
	return st.Size(), f.Close()
}

func writeJSON(path string, v any, indent bool) error {
	var (
		b   []byte
		err error
	)
	if indent {
		b, err = json.MarshalIndent(v, "", "  ")
	} else {
		b, err = json.Marshal(v)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func roundTo(v float64, decimals int) float64 {
	scale := math.Pow10(decimals)
	return math.Round(v*scale) / scale
}
