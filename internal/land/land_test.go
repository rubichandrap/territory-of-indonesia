package land

import (
	"testing"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"

	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
	"github.com/rubichandrap/territory-of-indonesia/internal/hierarchy"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
	. "github.com/rubichandrap/territory-of-indonesia/internal/testutil"
)

func sq(x0, y0, x1, y1 float64) orb.Polygon {
	return geom.Rewind(orb.Polygon{Rect(x0, y0, x1, y1)}).(orb.Polygon)
}

func TestUnionMergesTouchingRegionsIntoOneOutline(t *testing.T) {
	u := Union([]orb.Polygon{sq(0, 0, 1, 1), sq(1, 0, 2, 1)})
	p, ok := u.(orb.Polygon)
	if !ok || len(p) != 1 {
		t.Fatalf("want one polygon without holes, got %#v", u)
	}
	if a := planar.Area(p); a != 2 {
		t.Errorf("area = %v, want 2", a)
	}
	if o := p[0].Orientation(); o != orb.CCW {
		t.Errorf("exterior ring must be counter-clockwise, got %v", o)
	}
}

func TestUnionFillsAnEnclave(t *testing.T) {
	// A kabupaten with a hole, and the kota that fills that hole.
	kab := geom.Rewind(orb.Polygon{Rect(0, 0, 10, 10), Rect(4, 4, 6, 6)}).(orb.Polygon)
	kota := sq(4, 4, 6, 6)
	p, ok := Union([]orb.Polygon{kab, kota}).(orb.Polygon)
	if !ok || len(p) != 1 {
		t.Fatalf("the enclave must disappear, got %#v", p)
	}
	if a := planar.Area(p); a != 100 {
		t.Errorf("area = %v, want 100", a)
	}
}

func TestUnionKeepsIslandsAndRealHoles(t *testing.T) {
	// Two separate islands, and a ring of land around a lake.
	u := Union([]orb.Polygon{sq(0, 0, 1, 1), sq(5, 5, 6, 6), geom.Rewind(orb.Polygon{Rect(10, 10, 20, 20), Rect(14, 14, 16, 16)}).(orb.Polygon)})
	mp, ok := u.(orb.MultiPolygon)
	if !ok || len(mp) != 3 {
		t.Fatalf("want 3 polygons, got %#v", u)
	}
	holes := 0
	for _, p := range mp {
		holes += len(p) - 1
	}
	if holes != 1 {
		t.Errorf("the lake must stay a hole, got %d holes", holes)
	}
	if geom.Validate(u) != "" {
		t.Errorf("invalid result: %s", geom.Validate(u))
	}
}

func TestUnionIsDeterministic(t *testing.T) {
	in := []orb.Polygon{sq(0, 0, 1, 1), sq(1, 0, 2, 1), sq(0, 1, 2, 2)}
	a, b := Union(in), Union(in)
	if !orb.Equal(a, b) {
		t.Error("two unions of the same input differ")
	}
}

func TestProvincesToLandReplacesSeaWithTheRegencies(t *testing.T) {
	// A province whose polygon is mostly sea, with two regencies on its coast.
	prov := Region(1, model.Province, "P", "11", orb.Polygon{Rect(0, 0, 100, 100)})
	a := Region(2, model.Regency, "A", "11.01", sq(10, 10, 20, 20))
	b := Region(3, model.Regency, "B", "11.02", sq(20, 10, 30, 20))
	tree := hierarchy.Build([]*model.Region{prov, a, b})

	res := ProvincesToLand(tree)

	if len(res) != 1 || res[0].Unchanged {
		t.Fatalf("results = %+v", res)
	}
	if got := geom.Area(prov.Geometry); got != 200 {
		t.Errorf("province area = %v, want the 200 of its two regencies", got)
	}
	if s := res[0].SeaShare; s < 0.97 || s > 0.99 {
		t.Errorf("sea share = %v, want about 0.98", s)
	}
	if !prov.Bound.Contains(prov.Center) || !geom.Contains(prov.Geometry, prov.Center) {
		t.Errorf("centre %v is not on the land", prov.Center)
	}
}

func TestProvincesToLandLeavesAProvinceWithoutRegencies(t *testing.T) {
	prov := Region(1, model.Province, "P", "11", orb.Polygon{Rect(0, 0, 100, 100)})
	tree := hierarchy.Build([]*model.Region{prov})
	res := ProvincesToLand(tree)
	if len(res) != 1 || !res[0].Unchanged || geom.Area(prov.Geometry) != 10000 {
		t.Errorf("a province with no regencies must keep its own polygon: %+v", res)
	}
}
