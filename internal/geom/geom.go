// Package geom holds the small amount of planar geometry the build needs:
// winding, interior points, containment, simplification and rounding.
//
// All coordinates are WGS84 degrees treated as planar. That is wrong for
// distances and areas in absolute terms, but it is fine for what this package
// is used for: ranking candidates and drawing maps.
package geom

import (
	"math"
	"sort"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"
	"github.com/paulmach/orb/simplify"
)

// Polygons returns the polygons of a Polygon or MultiPolygon, nil otherwise.
func Polygons(g orb.Geometry) []orb.Polygon {
	switch v := g.(type) {
	case orb.Polygon:
		return []orb.Polygon{v}
	case orb.MultiPolygon:
		return v
	}
	return nil
}

// Contains reports whether pt is inside g. Points inside a hole are outside.
func Contains(g orb.Geometry, pt orb.Point) bool {
	switch v := g.(type) {
	case orb.Polygon:
		return planar.PolygonContains(v, pt)
	case orb.MultiPolygon:
		return planar.MultiPolygonContains(v, pt)
	}
	return false
}

// Rewind enforces RFC 7946 winding in place: exterior rings counter-clockwise,
// holes clockwise.
func Rewind(g orb.Geometry) orb.Geometry {
	for _, p := range Polygons(g) {
		for i, r := range p {
			want := orb.CCW
			if i > 0 {
				want = orb.CW
			}
			if o := r.Orientation(); o != 0 && o != want {
				r.Reverse()
			}
		}
	}
	return g
}

// Validate returns a description of the first structural problem in g, or "".
// It checks what GeoJSON consumers choke on, not full OGC validity.
func Validate(g orb.Geometry) string {
	polys := Polygons(g)
	if len(polys) == 0 {
		return "geometry is not a Polygon or MultiPolygon"
	}
	for _, p := range polys {
		if len(p) == 0 {
			return "polygon without rings"
		}
		for _, r := range p {
			if len(r) < 4 {
				return "ring with fewer than 4 points"
			}
			if r[0] != r[len(r)-1] {
				return "ring is not closed"
			}
			for _, pt := range r {
				if math.IsNaN(pt[0]) || math.IsNaN(pt[1]) || math.IsInf(pt[0], 0) || math.IsInf(pt[1], 0) {
					return "non-finite coordinate"
				}
			}
		}
	}
	return ""
}

// Area is the planar area of g in square degrees.
func Area(g orb.Geometry) float64 { return planar.Area(g) }

// InteriorPoint returns a point that lies inside the largest polygon of g. It
// scans a few horizontal lines through the polygon and takes the middle of the
// widest stretch that is inside, so unlike a centroid it is never in a hole or
// in the sea beside a crescent-shaped island.
func InteriorPoint(g orb.Geometry) (orb.Point, bool) {
	polys := Polygons(g)
	if len(polys) == 0 {
		return orb.Point{}, false
	}
	best, bestArea := polys[0], planar.Area(polys[0])
	for _, p := range polys[1:] {
		if a := planar.Area(p); a > bestArea {
			best, bestArea = p, a
		}
	}
	if len(best) == 0 || len(best[0]) < 4 {
		return orb.Point{}, false
	}
	b := best.Bound()
	var (
		pt     orb.Point
		widest float64
	)
	const scans = 9
	for i := 1; i <= scans; i++ {
		y := b.Min[1] + (b.Max[1]-b.Min[1])*float64(i)/float64(scans+1)
		xs := crossings(best, y)
		for j := 0; j+1 < len(xs); j += 2 {
			if w := xs[j+1] - xs[j]; w > widest {
				widest = w
				pt = orb.Point{(xs[j] + xs[j+1]) / 2, y}
			}
		}
	}
	if widest == 0 || !planar.PolygonContains(best, pt) {
		return orb.Point{}, false
	}
	return pt, true
}

// crossings returns the sorted x positions where the horizontal line at y
// crosses any ring of p. Edges use a half-open rule so a line through a vertex
// is counted once and every closed ring contributes an even number.
func crossings(p orb.Polygon, y float64) []float64 {
	var xs []float64
	for _, ring := range p {
		for i := 0; i+1 < len(ring); i++ {
			a, b := ring[i], ring[i+1]
			if (a[1] <= y) == (b[1] <= y) {
				continue
			}
			t := (y - a[1]) / (b[1] - a[1])
			xs = append(xs, a[0]+t*(b[0]-a[0]))
		}
	}
	sort.Float64s(xs)
	return xs
}

// Simplify returns a Douglas-Peucker simplified copy of g. The input is not
// modified.
//
// The tolerance is capped at a tenth of each polygon's own size, so a small
// island or village is simplified relative to its size instead of collapsing,
// and no polygon is ever dropped: an archipelago keeps all its islands.
// Simplification is applied per ring, so borders shared with a neighbour are
// not guaranteed to stay shared. It is for drawing, not for analysis.
func Simplify(g orb.Geometry, tolerance float64) orb.Geometry {
	if tolerance <= 0 {
		return orb.Clone(g)
	}
	var out orb.MultiPolygon
	for _, p := range Polygons(orb.Clone(g)) {
		b := p.Bound()
		size := math.Max(b.Max[0]-b.Min[0], b.Max[1]-b.Min[1])
		s := simplify.DouglasPeucker(math.Min(tolerance, size/10))
		sp := make(orb.Polygon, 0, len(p))
		for i, r := range p {
			orig := r.Clone()
			r = s.Ring(r)
			if len(r) < 4 {
				if i == 0 {
					r = orig // never lose a polygon
				} else {
					continue // a hole that collapsed is gone
				}
			}
			sp = append(sp, r)
		}
		out = append(out, sp)
	}
	if len(out) == 1 {
		return out[0]
	}
	return out
}

// Round returns a copy of g with every coordinate rounded to the given number
// of decimals and consecutive duplicate points removed. Rings that collapse
// below 4 points are removed; check the result with Validate.
func Round(g orb.Geometry, decimals int) orb.Geometry {
	g = orb.Clone(g)
	scale := math.Pow10(decimals)
	rnd := func(v float64) float64 { return math.Round(v*scale) / scale }
	polys := Polygons(g)
	for pi, p := range polys {
		rings := p[:0]
		for ri, r := range p {
			out := r[:0]
			for _, pt := range r {
				pt = orb.Point{rnd(pt[0]), rnd(pt[1])}
				if n := len(out); n > 0 && out[n-1] == pt {
					continue
				}
				out = append(out, pt)
			}
			if len(out) < 4 || out[0] != out[len(out)-1] {
				if ri == 0 {
					rings = nil
					break
				}
				continue
			}
			rings = append(rings, out)
		}
		polys[pi] = rings
	}
	switch v := g.(type) {
	case orb.Polygon:
		return polys[0]
	case orb.MultiPolygon:
		kept := v[:0]
		for _, p := range polys {
			if len(p) > 0 {
				kept = append(kept, p)
			}
		}
		return kept
	}
	return g
}
