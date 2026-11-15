package emit

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/paulmach/orb"

	"github.com/rubichandrap/territory-of-indonesia/internal/hierarchy"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
	. "github.com/rubichandrap/territory-of-indonesia/internal/testutil"
)

// wiggly is a square whose bottom edge has many tiny bumps, so simplification
// has something to remove.
func wiggly(x0, y0, x1, y1 float64) orb.Polygon {
	ring := orb.Ring{{x0, y0}}
	const n = 200
	for i := 1; i < n; i++ {
		x := x0 + (x1-x0)*float64(i)/n
		y := y0
		if i%2 == 0 {
			y += 0.0000123
		}
		ring = append(ring, orb.Point{x, y})
	}
	ring = append(ring, orb.Point{x1, y0}, orb.Point{x1, y1}, orb.Point{x0, y1}, orb.Point{x0, y0})
	return orb.Polygon{ring}
}

func sampleTree() *hierarchy.Tree {
	prov := Region(100, model.Province, "Jawa Barat", "32", wiggly(100, -8, 110, 0))
	reg := Region(200, model.Regency, "Kota Bandung", "32.73", wiggly(101, -7, 105, -3))
	dist := Region(300, model.District, "Antapani", "32.73.01", wiggly(102, -6, 103, -5))
	vill := Region(400, model.Village, "Antapani Kidul", "32.73.01.1001", wiggly(102.1, -5.9, 102.2, -5.8))
	orphanVill := Region(401, model.Village, "Tanpa Kecamatan", "", wiggly(104, -4, 104.5, -3.5)) // no district around it
	return hierarchy.Build([]*model.Region{prov, reg, dist, vill, orphanVill})
}

func TestWriteProducesTheDocumentedLayout(t *testing.T) {
	dir := t.TempDir()
	m, err := Write(sampleTree(), Options{Dir: dir, Source: Source{Name: "OpenStreetMap"}, Tool: "test"})
	if err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		"manifest.json",
		"index/provinces.json", "index/regencies/100.json", "index/districts/200.json",
		"index/villages/300.json", "index/villages/200.json", // the second: village attached to a regency
		"simplified/provinces.geojson", "simplified/villages/300.geojson",
		"full/provinces.geojson", "full/regencies/100.geojson",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing %s", rel)
		}
	}

	if m.Counts["provinces"] != 1 || m.Counts["villages"] != 2 || m.SchemaVersion != 2 {
		t.Errorf("manifest = %+v", m)
	}
	if m.Tiers[TierIndex].Files != 5 || m.Tiers[TierFull].Files != 5 {
		t.Errorf("tier file counts = %+v", m.Tiers)
	}

	// The regency's index entry lists both kinds of children.
	var entries []IndexEntry
	readJSON(t, filepath.Join(dir, "index/regencies/100.json"), &entries)
	if len(entries) != 1 || entries[0].OSMID != 200 || entries[0].Kind != "" && entries[0].Kind != "kota" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
	if got := entries[0].Counts; got["districts"] != 1 || got["villages"] != 1 {
		t.Errorf("children = %v, want 1 district and 1 village", got)
	}
	if entries[0].ParentID != 100 || len(entries[0].Ancestors) != 1 {
		t.Errorf("parent/ancestors = %d %v", entries[0].ParentID, entries[0].Ancestors)
	}
}

func TestWriteGeoJSONIsValidAndSimplifiedIsSmaller(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(sampleTree(), Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}

	var full, simple struct {
		Type     string `json:"type"`
		Features []struct {
			ID         int64          `json:"id"`
			BBox       []float64      `json:"bbox"`
			Properties map[string]any `json:"properties"`
			Geometry   struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	readJSON(t, filepath.Join(dir, "full/regencies/100.geojson"), &full)
	readJSON(t, filepath.Join(dir, "simplified/regencies/100.geojson"), &simple)

	if full.Type != "FeatureCollection" || len(full.Features) != 1 {
		t.Fatalf("unexpected collection: %+v", full)
	}
	f := full.Features[0]
	if f.ID != 200 || f.Properties["osm_id"].(float64) != 200 || f.Properties["level"] != "regency" || len(f.BBox) != 4 {
		t.Errorf("unexpected feature: %+v", f)
	}
	if len(simple.Features[0].Geometry.Coordinates) >= len(f.Geometry.Coordinates) {
		t.Errorf("simplified geometry (%d bytes) is not smaller than full (%d bytes)",
			len(simple.Features[0].Geometry.Coordinates), len(f.Geometry.Coordinates))
	}
}

func TestWriteIsByteForByteReproducible(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, dir := range []string{a, b} {
		if _, err := Write(sampleTree(), Options{Dir: dir}); err != nil {
			t.Fatal(err)
		}
	}
	fa, fb := snapshot(t, a), snapshot(t, b)
	if len(fa) == 0 || len(fa) != len(fb) {
		t.Fatalf("file sets differ: %d vs %d", len(fa), len(fb))
	}
	for name, content := range fa {
		if !bytes.Equal(content, fb[name]) {
			t.Errorf("%s differs between runs", name)
		}
	}
}

func TestWriteReplacesOldOutputButKeepsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "full", "villages", "999.geojson")
	keep := filepath.Join(dir, "README.txt")
	for _, p := range []string{stale, keep} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Write(sampleTree(), Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("stale output from a previous run survived")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("an unrelated file was deleted")
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func snapshot(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[rel] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
