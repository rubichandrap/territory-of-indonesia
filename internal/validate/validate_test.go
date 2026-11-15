package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paulmach/orb"

	"github.com/rubichandrap/territory-of-indonesia/internal/emit"
	"github.com/rubichandrap/territory-of-indonesia/internal/hierarchy"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
	. "github.com/rubichandrap/territory-of-indonesia/internal/testutil"
)

func p(x0, y0, x1, y1 float64) orb.Polygon { return orb.Polygon{Rect(x0, y0, x1, y1)} }

// build writes a valid three-level dataset into a temp dir.
func build(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tree := hierarchy.Build([]*model.Region{
		Region(1, model.Province, "Jawa Barat", "32", p(100, -8, 110, 0)),
		Region(2, model.Regency, "Kota Bandung", "32.73", p(101, -7, 105, -3)),
		Region(3, model.District, "Antapani", "32.73.01", p(102, -6, 103, -5)),
	})
	src := emit.Source{
		Name: "OpenStreetMap", Provider: "Geofabrik", Timestamp: "2026-11-14T00:00:00Z",
		Extracts: []emit.Extract{{File: "x.osm.pbf", SHA256: "abc", Timestamp: "2026-11-14T00:00:00Z"}},
	}
	if _, err := emit.Write(tree, emit.Options{Dir: dir, Source: src}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func checks(r *Result, sev Severity) []string {
	var out []string
	for _, f := range r.Findings {
		if f.Severity == sev {
			out = append(out, f.Check)
		}
	}
	return out
}

func TestValidDatasetPasses(t *testing.T) {
	res, err := Run(Options{Dir: build(t)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Errors() != 0 {
		t.Fatalf("unexpected findings: %+v", res.Findings)
	}
	if res.Counts["provinces"] != 1 || res.Counts["regencies"] != 1 || res.Counts["districts"] != 1 {
		t.Errorf("counts = %v", res.Counts)
	}
	if res.Files["index"] != 3 || res.Files["full"] != 3 || res.Files["simplified"] != 3 {
		t.Errorf("files = %v", res.Files)
	}
}

func TestDetectsBrokenDatasets(t *testing.T) {
	tests := []struct {
		name   string
		break_ func(t *testing.T, dir string)
		want   string // check name that must be reported
	}{
		{"missing geojson file", func(t *testing.T, dir string) {
			rm(t, dir, "full/regencies/1.geojson")
		}, "tier-missing-file"},
		{"extra geojson file", func(t *testing.T, dir string) {
			write(t, dir, "simplified/villages/999.geojson", `{"type":"FeatureCollection","features":[]}`)
		}, "tier-extra-file"},
		{"truncated geojson", func(t *testing.T, dir string) {
			write(t, dir, "full/provinces.geojson", `{"type":"FeatureCollection","features":[`)
		}, "geojson"},
		{"feature with an open ring", func(t *testing.T, dir string) {
			write(t, dir, "full/provinces.geojson", `{"type":"FeatureCollection","features":[{"type":"Feature","id":1,"properties":{"osm_id":1},"geometry":{"type":"Polygon","coordinates":[[[100,-8],[110,-8],[110,0],[100,0]]]}}]}`)
		}, "geometry"},
		{"geometry outside Indonesia", func(t *testing.T, dir string) {
			write(t, dir, "full/provinces.geojson", `{"type":"FeatureCollection","features":[{"type":"Feature","id":1,"properties":{"osm_id":1},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}]}`)
		}, "geometry-bounds"},
		{"index entry for a parent that does not exist", func(t *testing.T, dir string) {
			mv(t, dir, "index/districts/2.json", "index/districts/77.json")
		}, "parent-missing"},
		{"children count lies", func(t *testing.T, dir string) {
			replace(t, dir, "index/regencies/1.json", `"districts":1`, `"districts":4`)
		}, "children-count"},
		{"unknown field in the index", func(t *testing.T, dir string) {
			replace(t, dir, "index/provinces.json", `"osm_id":1,`, `"osm_id":1,"surprise":true,`)
		}, "index"},
		{"manifest count wrong", func(t *testing.T, dir string) {
			replace(t, dir, "manifest.json", `"villages": 0`, `"villages": 5`)
		}, "manifest-counts"},
		{"missing manifest", func(t *testing.T, dir string) {
			rm(t, dir, "manifest.json")
		}, "manifest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := build(t)
			tt.break_(t, dir)
			res, err := Run(Options{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			got := checks(res, Error)
			for _, c := range got {
				if c == tt.want {
					return
				}
			}
			t.Errorf("expected an error from check %q, got %v", tt.want, got)
		})
	}
}

func TestBaselineRegression(t *testing.T) {
	dir := build(t)
	bl := &Baseline{Counts: map[string]int{"regencies": 100}}

	res, _ := Run(Options{Dir: dir, Baseline: bl, MaxDrop: 0.02})
	if !contains(checks(res, Error), "regression") {
		t.Errorf("a drop from 100 to 1 should fail: %+v", res.Findings)
	}

	res, _ = Run(Options{Dir: dir, Baseline: &Baseline{Counts: map[string]int{"regencies": 1}}, MaxDrop: 0.02})
	if res.Errors() != 0 {
		t.Errorf("same count must pass: %+v", res.Findings)
	}
}

func TestRunRejectsMissingDirectory(t *testing.T) {
	if _, err := Run(Options{Dir: filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Error("expected an error for a missing directory")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func rm(t *testing.T, dir, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

func mv(t *testing.T, dir, from, to string) {
	t.Helper()
	if err := os.Rename(filepath.Join(dir, filepath.FromSlash(from)), filepath.Join(dir, filepath.FromSlash(to))); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replace(t *testing.T, dir, rel, old, new string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s does not contain %q:\n%s", rel, old, b)
	}
	write(t, dir, rel, strings.Replace(string(b), old, new, 1))
}
