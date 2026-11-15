// Package source reads the GeoJSON text sequence that `osmium export` writes
// and turns administrative boundary features into model.Region values.
package source

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/paulmach/orb"
	"github.com/rubichandrap/territory-of-indonesia/internal/geojson"
	"github.com/rubichandrap/territory-of-indonesia/internal/geom"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
)

// Stats counts what the reader saw and why it skipped features.
type Stats struct {
	Lines   int            // non-empty input lines
	Kept    int            // regions returned
	Skipped map[string]int // reason -> count
}

func (s *Stats) skip(reason string) {
	if s.Skipped == nil {
		s.Skipped = map[string]int{}
	}
	s.Skipped[reason]++
}

// Skip reasons.
const (
	SkipBadJSON       = "unparseable feature"
	SkipNotRelation   = "not a relation (way or node polygon)"
	SkipNotAdmin      = "not boundary=administrative"
	SkipLevel         = "admin_level outside 4-7"
	SkipGeometry      = "geometry is not a polygon"
	SkipInvalidGeom   = "invalid geometry"
	SkipNoInteriorPnt = "no interior point"
)

// maxLine bounds a single feature. The biggest provinces are a few MB.
const maxLine = 256 << 20

// Read parses a GeoJSON text sequence (one feature per line, optionally
// prefixed with the RS character) and returns the administrative regions in
// it. Features that are not relations, not boundary=administrative or not at a
// published level are counted in Stats and dropped.
func Read(r io.Reader) ([]*model.Region, Stats, error) {
	var (
		st      Stats
		regions []*model.Region
	)
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			st.Lines++
			reg, reason := parseFeature(line)
			if reason != "" {
				st.skip(reason)
			} else {
				regions = append(regions, reg)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, st, err
		}
	}
	st.Kept = len(regions)
	return regions, st, nil
}

// readLine returns the next line without its terminator or a leading RS
// (0x1e) character. It returns io.EOF together with the final line when the
// input does not end in a newline.
func readLine(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxLine {
			return nil, fmt.Errorf("feature longer than %d bytes", maxLine)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		buf = bytes.TrimSpace(bytes.TrimPrefix(buf, []byte{0x1e}))
		return buf, err
	}
}

func parseFeature(line []byte) (*model.Region, string) {
	f, err := geojson.ParseFeature(line)
	if err != nil {
		return nil, SkipBadJSON
	}
	id, ok := relationID(f)
	if !ok {
		return nil, SkipNotRelation
	}
	if str(f.Properties, "boundary") != "administrative" {
		return nil, SkipNotAdmin
	}
	lvl, err := strconv.Atoi(strings.TrimSpace(str(f.Properties, "admin_level")))
	if err != nil || !model.Level(lvl).Valid() {
		return nil, SkipLevel
	}
	level := model.Level(lvl)
	if f.Geometry == nil {
		return nil, SkipGeometry
	}
	g := geom.Rewind(f.Geometry)
	// osmium writes every area as a MultiPolygon; a single polygon is
	// published as a Polygon.
	if mp, ok := g.(orb.MultiPolygon); ok && len(mp) == 1 {
		g = mp[0]
	}
	if geom.Validate(g) != "" {
		return nil, SkipInvalidGeom
	}
	center, ok := geom.InteriorPoint(g)
	if !ok {
		return nil, SkipNoInteriorPnt
	}

	p := f.Properties
	code := ""
	for _, key := range []string{"ref:ID:kemendagri", "ref:kemendagri", "ref"} {
		if code = model.NormalizeCode(level, str(p, key)); code != "" {
			break
		}
	}
	reg := &model.Region{
		OSMID:        id,
		Level:        level,
		Name:         strings.TrimSpace(str(p, "name")),
		NameEN:       strings.TrimSpace(str(p, "name:en")),
		OfficialName: strings.TrimSpace(str(p, "official_name")),
		Wikidata:     strings.TrimSpace(str(p, "wikidata")),
		Code:         code,
		Geometry:     g,
		Bound:        g.Bound(),
		Area:         geom.Area(g),
		Center:       center,
	}
	if level == model.Province {
		reg.ISO3166_2 = strings.TrimSpace(str(p, "ISO3166-2"))
	}
	reg.Kind = model.DeriveKind(level, reg.OfficialName, reg.Name, reg.Code)
	return reg, ""
}

// relationID extracts the OSM relation id from what osmium export writes.
//
// osmium exports polygons as areas, so with --add-unique-id=type_id the id is
// "a<area id>". An area made from a relation has the odd id 2*relation+1, one
// made from a closed way the even id 2*way. "r123" and a bare number are
// accepted too, for hand-made fixtures.
func relationID(f *geojson.Feature) (int64, bool) {
	switch v := f.ID.(type) {
	case string:
		if rest, ok := strings.CutPrefix(v, "a"); ok {
			n, err := strconv.ParseInt(rest, 10, 64)
			if err != nil || n <= 0 || n%2 == 0 {
				return 0, false
			}
			return (n - 1) / 2, true
		}
		if rest, ok := strings.CutPrefix(v, "r"); ok {
			n, err := strconv.ParseInt(rest, 10, 64)
			return n, err == nil && n > 0
		}
		return 0, false
	case float64:
		n := int64(v)
		return n, n > 0
	}
	return 0, false
}

func str(p map[string]any, key string) string {
	switch v := p[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}
