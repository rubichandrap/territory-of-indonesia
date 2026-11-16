package land

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/paulmach/orb"

	"github.com/rubichandrap/territory-of-indonesia/internal/geojson"
	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
)

// TestRealDatasetUnions checks Union against a built dataset: set TOI_DIST to a
// dist/ from `toi build`. It is skipped otherwise.
//
// For every province the regencies must union into valid land whose area is
// close to the sum of the regencies' areas. A big shortfall would mean borders
// that do not match vertex for vertex, which Union cannot cancel.
func TestRealDatasetUnions(t *testing.T) {
	dist := os.Getenv("TOI_DIST")
	if dist == "" {
		t.Skip("set TOI_DIST to a built dist/ to run this")
	}
	var provinces []struct {
		OSMID int64  `json:"osm_id"`
		Name  string `json:"name"`
	}
	b, err := os.ReadFile(filepath.Join(dist, "index", "provinces.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &provinces); err != nil {
		t.Fatal(err)
	}
	for _, p := range provinces {
		raw, err := os.ReadFile(filepath.Join(dist, "full", "regencies", strconv.FormatInt(p.OSMID, 10)+".geojson"))
		if err != nil {
			t.Errorf("%s: %v", p.Name, err)
			continue
		}
		var fc struct {
			Features []json.RawMessage `json:"features"`
		}
		if err := json.Unmarshal(raw, &fc); err != nil {
			t.Fatal(err)
		}
		var polys []orb.Polygon
		var sum float64
		for _, rf := range fc.Features {
			f, err := geojson.ParseFeature(rf)
			if err != nil {
				t.Fatal(err)
			}
			polys = append(polys, geom.Polygons(f.Geometry)...)
			sum += geom.Area(f.Geometry)
		}
		u := Union(polys)
		if u == nil {
			t.Errorf("%s: empty union", p.Name)
			continue
		}
		if msg := geom.Validate(u); msg != "" {
			t.Errorf("%s: %s", p.Name, msg)
		}
		ratio := geom.Area(u) / sum
		t.Logf("%-32s regencies %2d  polygons %4d  area/sum %.4f", p.Name, len(fc.Features), len(geom.Polygons(u)), ratio)
		if ratio < 0.97 || ratio > 1.0001 {
			t.Errorf("%s: union area is %.4f of the regencies' total, expected about 1", p.Name, ratio)
		}
	}
}
