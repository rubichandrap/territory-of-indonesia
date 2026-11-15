package source

import (
	"strings"
	"testing"

	"github.com/paulmach/orb"
)

const square = `{"type":"Polygon","coordinates":[[[0,0],[10,0],[10,10],[0,10],[0,0]]]}`

func feature(id, props, geometry string) string {
	return `{"type":"Feature","id":"` + id + `","properties":` + props + `,"geometry":` + geometry + `}`
}

func TestReadKeepsAdministrativeRelationsAndCountsTheRest(t *testing.T) {
	admin := `{"boundary":"administrative","admin_level":"5","name":"Bandung","official_name":"Kota Bandung","ref:ID:kemendagri":"32.73","wikidata":"Q1"}`
	lines := []string{
		// RS-prefixed record, as osmium writes by default.
		"\x1e" + feature("a201", admin, square), // odd area id: relation 100
		feature("a10", admin, square),           // even area id: closed way 5
		feature("r101", `{"boundary":"protected_area","admin_level":"5"}`, square),
		feature("r102", `{"boundary":"administrative","admin_level":"8"}`, square),
		feature("r103", `{"boundary":"administrative","admin_level":"5"}`, `{"type":"Point","coordinates":[0,0]}`),
		"not json at all",
		"",
	}

	regs, st, err := Read(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(regs) != 1 {
		t.Fatalf("kept %d regions, want 1", len(regs))
	}
	r := regs[0]
	if r.OSMID != 100 || r.Level != 5 || r.Name != "Bandung" || r.Code != "32.73" || r.Kind != "kota" || r.Wikidata != "Q1" {
		t.Errorf("unexpected region: %+v", r)
	}
	if !r.Bound.Contains(r.Center) {
		t.Errorf("interior point %v outside bound %v", r.Center, r.Bound)
	}
	want := map[string]int{SkipNotRelation: 1, SkipNotAdmin: 1, SkipLevel: 1, SkipGeometry: 1, SkipBadJSON: 1}
	for reason, n := range want {
		if st.Skipped[reason] != n {
			t.Errorf("skipped[%q] = %d, want %d", reason, st.Skipped[reason], n)
		}
	}
	if st.Lines != 6 || st.Kept != 1 {
		t.Errorf("stats = %+v", st)
	}
}

func TestReadFallsBackThroughCodeTags(t *testing.T) {
	props := `{"boundary":"administrative","admin_level":"6","ref":"32.73.01"}`
	regs, _, err := Read(strings.NewReader(feature("r1", props, square)))
	if err != nil || len(regs) != 1 {
		t.Fatalf("regs=%d err=%v", len(regs), err)
	}
	if regs[0].Code != "32.73.01" {
		t.Errorf("code = %q, want it taken from the ref tag", regs[0].Code)
	}

	// A ref with the wrong shape is not a code.
	props = `{"boundary":"administrative","admin_level":"4","ref":"JB"}`
	regs, _, _ = Read(strings.NewReader(feature("r2", props, square)))
	if regs[0].Code != "" {
		t.Errorf("code = %q, want empty for ref=JB", regs[0].Code)
	}
}

func TestReadHandlesFinalLineWithoutNewline(t *testing.T) {
	props := `{"boundary":"administrative","admin_level":"4","ISO3166-2":"ID-JB"}`
	regs, _, err := Read(strings.NewReader(feature("r7", props, square)))
	if err != nil || len(regs) != 1 || regs[0].ISO3166_2 != "ID-JB" {
		t.Fatalf("regs=%v err=%v", regs, err)
	}
}

func TestReadCollapsesSingleMultiPolygonAndKeepsRealOnes(t *testing.T) {
	props := `{"boundary":"administrative","admin_level":"5"}`
	one := `{"type":"MultiPolygon","coordinates":[[[[0,0],[10,0],[10,10],[0,10],[0,0]]]]}`
	two := `{"type":"MultiPolygon","coordinates":[[[[0,0],[1,0],[1,1],[0,1],[0,0]]],[[[5,5],[6,5],[6,6],[5,6],[5,5]]]]}`
	in := feature("a3", props, one) + "\n" + feature("a5", props, two)

	regs, _, err := Read(strings.NewReader(in))
	if err != nil || len(regs) != 2 {
		t.Fatalf("regs=%d err=%v", len(regs), err)
	}
	if _, ok := regs[0].Geometry.(orb.Polygon); !ok {
		t.Errorf("single-polygon MultiPolygon should become a Polygon, got %T", regs[0].Geometry)
	}
	if _, ok := regs[1].Geometry.(orb.MultiPolygon); !ok {
		t.Errorf("two-polygon MultiPolygon must stay a MultiPolygon, got %T", regs[1].Geometry)
	}
	if regs[0].OSMID != 1 || regs[1].OSMID != 2 {
		t.Errorf("area ids a3/a5 should map to relations 1 and 2, got %d and %d", regs[0].OSMID, regs[1].OSMID)
	}
}
