package geom

import (
	"testing"

	"github.com/paulmach/orb"
)

func rect(x0, y0, x1, y1 float64) orb.Ring {
	return orb.Ring{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}, {x0, y0}}
}

func TestRewindOrientsRingsPerRFC7946(t *testing.T) {
	outer := rect(0, 0, 10, 10)
	outer.Reverse()          // clockwise: wrong for an exterior ring
	hole := rect(2, 2, 4, 4) // counter-clockwise: wrong for a hole
	p := orb.Polygon{outer, hole}

	Rewind(p)

	if p[0].Orientation() != orb.CCW {
		t.Errorf("exterior ring should be counter-clockwise")
	}
	if p[1].Orientation() != orb.CW {
		t.Errorf("hole should be clockwise")
	}
}

func TestInteriorPointAvoidsHoles(t *testing.T) {
	// A square with a hole covering the centre, where a centroid would land.
	p := orb.Polygon{rect(0, 0, 10, 10), rect(2, 2, 8, 8)}
	pt, ok := InteriorPoint(p)
	if !ok {
		t.Fatal("no interior point")
	}
	if !Contains(p, pt) {
		t.Errorf("interior point %v is not inside the polygon", pt)
	}
}

func TestInteriorPointOfCrescent(t *testing.T) {
	// A C-shape: its bounding-box centre is outside the polygon.
	c := orb.Ring{{0, 0}, {10, 0}, {10, 2}, {2, 2}, {2, 8}, {10, 8}, {10, 10}, {0, 10}, {0, 0}}
	p := orb.Polygon{c}
	pt, ok := InteriorPoint(p)
	if !ok || !Contains(p, pt) {
		t.Fatalf("interior point %v (ok=%v) is not inside the C-shape", pt, ok)
	}
}

func TestInteriorPointUsesLargestPolygonOfMultiPolygon(t *testing.T) {
	big := orb.Polygon{rect(0, 0, 10, 10)}
	small := orb.Polygon{rect(20, 20, 21, 21)}
	pt, ok := InteriorPoint(orb.MultiPolygon{small, big})
	if !ok || !Contains(big, pt) {
		t.Errorf("expected a point in the larger polygon, got %v", pt)
	}
}

func TestValidate(t *testing.T) {
	if msg := Validate(orb.Polygon{rect(0, 0, 1, 1)}); msg != "" {
		t.Errorf("valid polygon rejected: %s", msg)
	}
	open := orb.Ring{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	if Validate(orb.Polygon{open}) == "" {
		t.Error("open ring accepted")
	}
	if Validate(orb.Point{1, 1}) == "" {
		t.Error("point accepted as an area")
	}
}

func TestSimplifyDoesNotModifyInput(t *testing.T) {
	ring := orb.Ring{{0, 0}, {5, 0.001}, {10, 0}, {10, 10}, {0, 10}, {0, 0}}
	in := orb.Polygon{ring}
	before := in.Clone()

	out := Simplify(in, 0.01)

	if !in.Equal(before) {
		t.Error("Simplify modified its input")
	}
	if n := len(out.(orb.Polygon)[0]); n >= len(ring) {
		t.Errorf("expected fewer points than %d, got %d", len(ring), n)
	}
}

func TestSimplifyKeepsEveryIslandAndScalesToSize(t *testing.T) {
	big := orb.Polygon{rect(0, 0, 10, 10)}
	// An island far smaller than the tolerance, with some detail.
	island := orb.Polygon{{{20, 20}, {20.0005, 20}, {20.001, 20.0004}, {20.001, 20.001}, {20.0004, 20.001}, {20, 20.0006}, {20, 20}}}

	out, ok := Simplify(orb.MultiPolygon{big, island}, 0.01).(orb.MultiPolygon)
	if !ok || len(out) != 2 {
		t.Fatalf("both polygons must survive, got %#v", out)
	}
	if Validate(out) != "" {
		t.Errorf("simplified geometry is invalid: %s", Validate(out))
	}
	if planar := Area(out[1]); planar < Area(island)*0.5 {
		t.Errorf("the island lost most of its area: %g of %g", planar, Area(island))
	}

	// Even a tolerance far above the whole polygon's size leaves a valid polygon.
	if Validate(Simplify(island, 1)) != "" {
		t.Error("simplifying a tiny polygon with a huge tolerance must still return a valid geometry")
	}
}

func TestRoundMergesDuplicatePointsAndClones(t *testing.T) {
	in := orb.Polygon{{{0, 0}, {0.00001, 0.00001}, {1, 0}, {1, 1}, {0, 1}, {0, 0}}}
	out := Round(in, 2).(orb.Polygon)

	if got := len(out[0]); got != 5 {
		t.Errorf("expected 5 points after rounding, got %d: %v", got, out[0])
	}
	if in[0][1] != (orb.Point{0.00001, 0.00001}) {
		t.Error("Round modified its input")
	}
}
