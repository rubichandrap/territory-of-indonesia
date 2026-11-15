// Package geojson is a small GeoJSON codec for what this project needs:
// reading Polygon and MultiPolygon features, and writing compact, deterministic
// features.
//
// It exists instead of github.com/paulmach/orb/geojson because that package
// pulls in the MongoDB driver for BSON support.
package geojson

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/paulmach/orb"
)

// Feature is a decoded GeoJSON feature.
type Feature struct {
	ID         any            // string or float64, whichever the input used
	Properties map[string]any // never nil
	// GeometryType is the "type" of the geometry as written; empty when the
	// feature has no geometry.
	GeometryType string
	// Geometry is set for Polygon and MultiPolygon only.
	Geometry orb.Geometry
}

// ParseFeature decodes one GeoJSON feature. Geometry types other than Polygon
// and MultiPolygon are reported through GeometryType and leave Geometry nil.
func ParseFeature(data []byte) (*Feature, error) {
	var raw struct {
		Type       string         `json:"type"`
		ID         any            `json:"id"`
		Properties map[string]any `json:"properties"`
		Geometry   *struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Type != "Feature" {
		return nil, fmt.Errorf("not a Feature: type %q", raw.Type)
	}
	f := &Feature{ID: raw.ID, Properties: raw.Properties}
	if f.Properties == nil {
		f.Properties = map[string]any{}
	}
	if raw.Geometry == nil {
		return f, nil
	}
	f.GeometryType = raw.Geometry.Type
	switch raw.Geometry.Type {
	case "Polygon":
		var p orb.Polygon
		if err := json.Unmarshal(raw.Geometry.Coordinates, &p); err != nil {
			return nil, fmt.Errorf("polygon coordinates: %w", err)
		}
		f.Geometry = p
	case "MultiPolygon":
		var mp orb.MultiPolygon
		if err := json.Unmarshal(raw.Geometry.Coordinates, &mp); err != nil {
			return nil, fmt.Errorf("multipolygon coordinates: %w", err)
		}
		f.Geometry = mp
	}
	return f, nil
}

// ErrUnsupported is returned by AppendGeometry for anything but polygons.
var ErrUnsupported = errors.New("geojson: only Polygon and MultiPolygon can be written")

// AppendGeometry appends the GeoJSON geometry object for g to dst.
// Coordinates are written with the shortest decimal that round-trips and never
// in exponent form, so output is compact and byte-for-byte reproducible.
func AppendGeometry(dst []byte, g orb.Geometry) ([]byte, error) {
	switch v := g.(type) {
	case orb.Polygon:
		dst = append(dst, `{"type":"Polygon","coordinates":`...)
		dst = appendPolygon(dst, v)
	case orb.MultiPolygon:
		dst = append(dst, `{"type":"MultiPolygon","coordinates":[`...)
		for i, p := range v {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendPolygon(dst, p)
		}
		dst = append(dst, ']')
	default:
		return dst, ErrUnsupported
	}
	return append(dst, '}'), nil
}

func appendPolygon(dst []byte, p orb.Polygon) []byte {
	dst = append(dst, '[')
	for i, r := range p {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, '[')
		for j, pt := range r {
			if j > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, '[')
			dst = strconv.AppendFloat(dst, pt[0], 'f', -1, 64)
			dst = append(dst, ',')
			dst = strconv.AppendFloat(dst, pt[1], 'f', -1, 64)
			dst = append(dst, ']')
		}
		dst = append(dst, ']')
	}
	return append(dst, ']')
}

// AppendBBox appends a GeoJSON bbox array [west,south,east,north].
func AppendBBox(dst []byte, b orb.Bound) []byte {
	dst = append(dst, '[')
	for i, v := range []float64{b.Min[0], b.Min[1], b.Max[0], b.Max[1]} {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = strconv.AppendFloat(dst, v, 'f', -1, 64)
	}
	return append(dst, ']')
}
