// Package testutil builds small synthetic regions for tests.
package testutil

import (
	"github.com/paulmach/orb"

	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
)

// Rect returns a closed counter-clockwise rectangle ring.
func Rect(x0, y0, x1, y1 float64) orb.Ring {
	return orb.Ring{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}, {x0, y0}}
}

// Region builds a region around g with all derived fields filled in.
func Region(id int64, level model.Level, name, code string, g orb.Geometry) *model.Region {
	g = geom.Rewind(g)
	c, ok := geom.InteriorPoint(g)
	if !ok {
		panic("testutil: no interior point for " + name)
	}
	r := &model.Region{
		OSMID:    id,
		Level:    level,
		Name:     name,
		Code:     code,
		Geometry: g,
		Bound:    g.Bound(),
		Area:     geom.Area(g),
		Center:   c,
	}
	if level == model.Province {
		r.ISO3166_2 = "ID-XX"
	}
	return r
}
