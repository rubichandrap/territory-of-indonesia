package geojson

import (
	"regexp"
	"testing"

	"github.com/paulmach/orb"
)

var exponent = regexp.MustCompile(`[0-9][eE][-+]?[0-9]`)

func TestParseFeaturePolygon(t *testing.T) {
	in := `{"type":"Feature","id":"r7","properties":{"name":"X"},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}`
	f, err := ParseFeature([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != "r7" || f.Properties["name"] != "X" || f.GeometryType != "Polygon" {
		t.Errorf("unexpected feature: %+v", f)
	}
	if p, ok := f.Geometry.(orb.Polygon); !ok || len(p[0]) != 4 {
		t.Errorf("unexpected geometry: %#v", f.Geometry)
	}
}

func TestParseFeatureKeepsTypeOfUnsupportedGeometry(t *testing.T) {
	in := `{"type":"Feature","properties":null,"geometry":{"type":"Point","coordinates":[1,2]}}`
	f, err := ParseFeature([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if f.Geometry != nil || f.GeometryType != "Point" || f.Properties == nil {
		t.Errorf("unexpected feature: %+v", f)
	}
}

func TestParseFeatureRejectsNonFeatures(t *testing.T) {
	for _, in := range []string{`{"type":"FeatureCollection"}`, `[1,2]`, `nope`} {
		if _, err := ParseFeature([]byte(in)); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

func TestAppendGeometryRoundTrip(t *testing.T) {
	mp := orb.MultiPolygon{
		{{{106.8271234, -6.1753924}, {106.9, -6.1}, {106.9, -6.2}, {106.8271234, -6.1753924}}},
		{{{0.0000001, 5}, {1, 5}, {1, 6}, {0.0000001, 5}}},
	}
	b, err := AppendGeometry(nil, mp)
	if err != nil {
		t.Fatal(err)
	}
	if exponent.Match(b) {
		t.Errorf("exponent form in output: %s", b)
	}
	f, err := ParseFeature([]byte(`{"type":"Feature","properties":{},"geometry":` + string(b) + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Geometry.(orb.MultiPolygon).Equal(mp) {
		t.Errorf("round trip changed the geometry:\n got %v\nwant %v", f.Geometry, mp)
	}
}

func TestAppendGeometryRejectsOtherTypes(t *testing.T) {
	if _, err := AppendGeometry(nil, orb.Point{1, 2}); err != ErrUnsupported {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}

func TestAppendBBox(t *testing.T) {
	got := string(AppendBBox(nil, orb.Bound{Min: orb.Point{95.1, -11}, Max: orb.Point{141, 6.25}}))
	if got != "[95.1,-11,141,6.25]" {
		t.Errorf("bbox = %s", got)
	}
}
