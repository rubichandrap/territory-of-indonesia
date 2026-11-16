// Package land turns province polygons into land.
//
// OpenStreetMap draws Indonesian province boundaries out into the territorial
// sea, so a province polygon is often mostly water: Kepulauan Riau is 97% sea
// and Maluku 92%. A map of those polygons has no coastline and no islands. The
// regencies inside a province are land, so the land of a province is the union
// of its regencies.
//
// Regencies share their borders vertex for vertex, because OpenStreetMap
// builds them from the same ways. That makes the union exact and cheap without
// a polygon clipping library: every shared border is walked once in each
// direction and cancels, and what is left is the outline.
package land

import (
	"sort"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"

	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
	"github.com/rubichandrap/territory-of-indonesia/internal/hierarchy"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
)

type edge struct{ a, b orb.Point }

// Union returns the union of polygons whose rings follow RFC 7946 winding
// (exteriors counter-clockwise, holes clockwise) and whose shared borders have
// identical vertices. Borders that do not match are not cancelled and show up
// as thin outlines, so the result is only as clean as the input is consistent.
// It returns nil when nothing is left.
func Union(polys []orb.Polygon) orb.Geometry {
	count := map[edge]int{}
	for _, p := range polys {
		for _, r := range p {
			for i := 0; i+1 < len(r); i++ {
				if r[i] != r[i+1] {
					count[edge{r[i], r[i+1]}]++
				}
			}
		}
	}
	// An edge cancels against the same edge walked the other way.
	next := map[orb.Point][]orb.Point{}
	keys := make([]edge, 0, len(count))
	for e := range count {
		keys = append(keys, e)
	}
	sort.Slice(keys, func(i, j int) bool { return less(keys[i], keys[j]) }) // deterministic output
	for _, e := range keys {
		n := count[e] - count[edge{e.b, e.a}]
		for ; n > 0; n-- {
			next[e.a] = append(next[e.a], e.b)
		}
	}

	var exteriors []orb.Ring
	var holes []orb.Ring
	for _, start := range sortedStarts(next) {
		for len(next[start]) > 0 {
			ring := walk(start, next)
			if len(ring) < 4 {
				continue
			}
			switch a := planar.Area(orb.Polygon{ring}); {
			case a == 0:
			case signed(ring) > 0:
				exteriors = append(exteriors, ring)
			default:
				holes = append(holes, ring)
			}
		}
	}
	return assemble(exteriors, holes)
}

// walk follows unused edges from start until it returns there, consuming them.
func walk(start orb.Point, next map[orb.Point][]orb.Point) orb.Ring {
	ring := orb.Ring{start}
	cur := start
	for {
		outs := next[cur]
		if len(outs) == 0 {
			return nil // an open chain: the input was not consistent
		}
		to := outs[0]
		next[cur] = outs[1:]
		ring = append(ring, to)
		if to == start {
			return ring
		}
		cur = to
	}
}

// assemble gives every hole to the smallest exterior that contains it.
func assemble(exteriors, holes []orb.Ring) orb.Geometry {
	if len(exteriors) == 0 {
		return nil
	}
	sort.SliceStable(exteriors, func(i, j int) bool {
		return planar.Area(orb.Polygon{exteriors[i]}) > planar.Area(orb.Polygon{exteriors[j]})
	})
	polys := make([]orb.Polygon, len(exteriors))
	for i, e := range exteriors {
		polys[i] = orb.Polygon{e}
	}
	for _, h := range holes {
		for i := len(polys) - 1; i >= 0; i-- { // smallest first
			if planar.RingContains(polys[i][0], h[0]) {
				polys[i] = append(polys[i], h)
				break
			}
		}
	}
	if len(polys) == 1 {
		return polys[0]
	}
	mp := make(orb.MultiPolygon, len(polys))
	copy(mp, polys)
	return mp
}

func signed(r orb.Ring) float64 {
	var a float64
	for i := 0; i+1 < len(r); i++ {
		a += r[i][0]*r[i+1][1] - r[i+1][0]*r[i][1]
	}
	return a / 2
}

func less(x, y edge) bool {
	if x.a != y.a {
		return pointLess(x.a, y.a)
	}
	return pointLess(x.b, y.b)
}

func pointLess(p, q orb.Point) bool {
	if p[0] != q[0] {
		return p[0] < q[0]
	}
	return p[1] < q[1]
}

func sortedStarts(next map[orb.Point][]orb.Point) []orb.Point {
	out := make([]orb.Point, 0, len(next))
	for p := range next {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return pointLess(out[i], out[j]) })
	return out
}

// Result says what ProvincesToLand did to one province.
type Result struct {
	Province  *model.Region
	SeaShare  float64 // fraction of the original polygon that was water
	Unchanged bool    // no regencies, or nothing usable came out of the union
}

// ProvincesToLand replaces the geometry of every province that has regencies
// with the land: the union of those regencies. Bound, Area and Center follow.
// Parents were resolved against the original polygons, so call this after
// hierarchy.Build.
func ProvincesToLand(t *hierarchy.Tree) []Result {
	var out []Result
	for _, p := range t.Children[0] {
		var polys []orb.Polygon
		for _, k := range t.Children[p.OSMID] {
			if k.Level == model.Regency {
				polys = append(polys, geom.Polygons(k.Geometry)...)
			}
		}
		res := Result{Province: p, Unchanged: true}
		if len(polys) > 0 {
			if u := Union(polys); u != nil && geom.Validate(u) == "" {
				if c, ok := geom.InteriorPoint(u); ok {
					before := p.Area
					p.Geometry, p.Bound, p.Area, p.Center = u, u.Bound(), geom.Area(u), c
					res.Unchanged = false
					if before > 0 {
						res.SeaShare = 1 - p.Area/before
					}
				}
			}
		}
		out = append(out, res)
	}
	return out
}
