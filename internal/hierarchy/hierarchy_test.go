package hierarchy

import (
	"testing"

	"github.com/paulmach/orb"

	"github.com/rubichandrap/territory-of-indonesia/internal/model"
	. "github.com/rubichandrap/territory-of-indonesia/internal/testutil"
)

func poly(x0, y0, x1, y1 float64) orb.Polygon { return orb.Polygon{Rect(x0, y0, x1, y1)} }

func issuesOf(t *Tree, kind IssueKind) []Issue {
	var out []Issue
	for _, i := range t.Issues {
		if i.Kind == kind {
			out = append(out, i)
		}
	}
	return out
}

func TestBuildAssignsParentsAndAncestors(t *testing.T) {
	prov := Region(1, model.Province, "Jawa Barat", "32", poly(0, 0, 100, 100))
	reg := Region(2, model.Regency, "Bandung", "32.73", poly(10, 10, 50, 50))
	dist := Region(3, model.District, "Antapani", "32.73.01", poly(20, 20, 30, 30))
	vill := Region(4, model.Village, "Antapani Kidul", "32.73.01.1001", poly(22, 22, 24, 24))

	tree := Build([]*model.Region{vill, dist, reg, prov}) // deliberately unordered

	if got := vill.ParentID; got != 3 {
		t.Errorf("village parent = %d, want 3", got)
	}
	if want := []int64{1, 2, 3}; !equal(vill.Ancestors, want) {
		t.Errorf("village ancestors = %v, want %v", vill.Ancestors, want)
	}
	if len(tree.Children[0]) != 1 || tree.Children[0][0] != prov {
		t.Errorf("provinces should be the children of 0, got %v", tree.Children[0])
	}
	if len(tree.Regions) != 4 {
		t.Errorf("kept %d regions, want 4", len(tree.Regions))
	}
	if len(tree.Issues) != 0 {
		t.Errorf("unexpected issues: %+v", tree.Issues)
	}
}

func TestBuildDropsRegionsOutsideIndonesia(t *testing.T) {
	prov := Region(1, model.Province, "Jawa Barat", "32", poly(0, 0, 100, 100))
	foreignProv := Region(9, model.Province, "Sabah", "", poly(200, 0, 300, 100))
	foreignProv.ISO3166_2 = "MY-12"
	foreignReg := Region(10, model.Regency, "Kudat", "", poly(210, 10, 250, 50))

	tree := Build([]*model.Region{prov, foreignProv, foreignReg})

	if len(tree.Regions) != 1 {
		t.Fatalf("kept %d regions, want only the Indonesian province", len(tree.Regions))
	}
	if n := len(issuesOf(tree, Outside)); n != 2 {
		t.Errorf("outside issues = %d, want 2", n)
	}
}

func TestBuildPicksTheEnclaveWhenParentsOverlap(t *testing.T) {
	// A kabupaten whose polygon also covers the kota inside it (no hole).
	prov := Region(1, model.Province, "P", "11", poly(0, 0, 100, 100))
	kab := Region(2, model.Regency, "Magelang", "11.08", poly(0, 0, 100, 100))
	kota := Region(3, model.Regency, "Kota Magelang", "11.71", poly(40, 40, 60, 60))
	kec := Region(4, model.District, "Magelang Tengah", "", poly(45, 45, 55, 55))

	tree := Build([]*model.Region{prov, kab, kota, kec})

	if kec.ParentID != 3 {
		t.Errorf("district parent = %d, want the kota (3)", kec.ParentID)
	}
	if n := len(issuesOf(tree, Ambiguous)); n != 1 {
		t.Errorf("ambiguous issues = %d, want 1", n)
	}
}

func TestBuildPrefersCodePrefixOverArea(t *testing.T) {
	// Two overlapping regencies; the code says the district belongs to the
	// bigger one, so smallest-area must not win.
	prov := Region(1, model.Province, "P", "11", poly(0, 0, 100, 100))
	big := Region(2, model.Regency, "Big", "11.01", poly(0, 0, 100, 100))
	small := Region(3, model.Regency, "Small", "11.02", poly(40, 40, 60, 60))
	kec := Region(4, model.District, "K", "11.01.05", poly(45, 45, 55, 55))

	Build([]*model.Region{prov, big, small, kec})

	if kec.ParentID != 2 {
		t.Errorf("district parent = %d, want 2 (matching code prefix)", kec.ParentID)
	}
}

func TestBuildAttachesToGrandparentWhenLevelIsMissing(t *testing.T) {
	prov := Region(1, model.Province, "P", "11", poly(0, 0, 100, 100))
	reg := Region(2, model.Regency, "R", "11.01", poly(0, 0, 50, 50))
	vill := Region(3, model.Village, "V", "", poly(10, 10, 12, 12)) // no district mapped

	tree := Build([]*model.Region{prov, reg, vill})

	if vill.ParentID != 2 {
		t.Fatalf("village parent = %d, want the regency (2)", vill.ParentID)
	}
	if n := tree.Stats[3].SkippedLevel; n != 1 {
		t.Errorf("village SkippedLevel = %d, want 1", n)
	}
	if got := tree.Children[2]; len(got) != 1 || got[0] != vill {
		t.Errorf("village should be listed under the regency, got %v", got)
	}
}

func TestBuildSkipsAtMostOneLevel(t *testing.T) {
	prov := Region(1, model.Province, "P", "11", poly(0, 0, 100, 100))
	// A district with no regency around it may hang off the province...
	dist := Region(2, model.District, "D", "", poly(10, 10, 20, 20))
	// ...but a village that only the province contains may not.
	vill := Region(3, model.Village, "V", "", poly(60, 60, 61, 61))
	// A village outside every province is simply foreign.
	far := Region(4, model.Village, "Far", "", poly(500, 500, 501, 501))

	tree := Build([]*model.Region{prov, dist, vill, far})

	if dist.ParentID != 1 {
		t.Errorf("district parent = %d, want the province", dist.ParentID)
	}
	if _, kept := tree.ByID[3]; kept {
		t.Error("a village that skips two levels must not be kept")
	}
	if n := len(issuesOf(tree, Orphan)); n != 1 {
		t.Errorf("orphan issues = %d, want 1 (the village inside the province)", n)
	}
	if n := len(issuesOf(tree, Outside)); n != 1 {
		t.Errorf("outside issues = %d, want 1 (the village outside every province)", n)
	}
	if st := tree.Stats[3]; st.Orphan != 1 || st.Outside != 1 || st.Kept != 0 {
		t.Errorf("village stats = %+v", st)
	}
}

func TestBuildReportsDuplicatesAndCodeMismatches(t *testing.T) {
	prov := Region(1, model.Province, "P", "11", poly(0, 0, 100, 100))
	dup := Region(1, model.Province, "P again", "11", poly(0, 0, 100, 100))
	reg := Region(2, model.Regency, "R", "99.01", poly(0, 0, 50, 50)) // wrong province code

	tree := Build([]*model.Region{prov, dup, reg})

	if n := len(issuesOf(tree, Duplicate)); n != 1 {
		t.Errorf("duplicate issues = %d, want 1", n)
	}
	if n := len(issuesOf(tree, CodeMismatch)); n != 1 {
		t.Errorf("code mismatch issues = %d, want 1", n)
	}
	st := tree.Stats[1]
	if st.CodeChecked != 1 || st.CodeConsistent != 0 {
		t.Errorf("regency code stats = %+v", st)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	mk := func() []*model.Region {
		return []*model.Region{
			Region(1, model.Province, "P", "11", poly(0, 0, 100, 100)),
			Region(2, model.Regency, "B", "11.01", poly(0, 0, 40, 40)),
			Region(3, model.Regency, "A", "11.02", poly(50, 50, 90, 90)),
		}
	}
	a, b := Build(mk()), Build(mk())
	if len(a.Children[1]) != 2 || a.Children[1][0].Name != "A" {
		t.Fatalf("children should be sorted by name, got %v", a.Children[1])
	}
	for i := range a.Regions {
		if a.Regions[i].OSMID != b.Regions[i].OSMID {
			t.Fatal("two builds of the same input differ")
		}
	}
}

// Regression for the v1 finding where Kampar Regency was filed directly under
// the country instead of its province, so the province index could not reach
// it or its districts. In v2 only levels 4-7 are published and provinces are
// roots, so a country-level region that geometrically contains everything is
// ignored: it can never become a parent, and the Kampar chain stays reachable.
// Geometry below is synthetic; the names and ids echo the real case only to
// make the failure concrete.
func TestBuildNeverParentsARegencyToCountry(t *testing.T) {
	country := &model.Region{
		OSMID:    100,
		Level:    2, // admin_level 2 is not published
		Name:     "Country",
		Geometry: poly(-10, -10, 200, 200),
	}
	prov := Region(2390840, model.Province, "Riau", "14", poly(0, 0, 100, 100))
	prov.ISO3166_2 = "ID-RI"
	reg := Region(7830592, model.Regency, "Kampar", "14.01", poly(10, 10, 50, 50))
	dist := Region(19390620, model.District, "Bangkinang", "14.01.01", poly(20, 20, 30, 30))

	tree := Build([]*model.Region{country, dist, reg, prov}) // country deliberately first

	if _, kept := tree.ByID[country.OSMID]; kept {
		t.Error("a country-level region must not be kept in the tree")
	}
	if len(tree.Regions) != 3 {
		t.Fatalf("kept %d regions, want 3 (province, regency, district)", len(tree.Regions))
	}
	if reg.ParentID != prov.OSMID {
		t.Errorf("Kampar parent = %d, want Riau (%d), never the country", reg.ParentID, prov.OSMID)
	}
	if want := []int64{prov.OSMID}; !equal(reg.Ancestors, want) {
		t.Errorf("Kampar ancestors = %v, want %v", reg.Ancestors, want)
	}

	// The whole chain must stay reachable from the province root down.
	if got := tree.Children[0]; len(got) != 1 || got[0] != prov {
		t.Errorf("regions under 0 = %v, want only Riau", got)
	}
	if got := tree.Children[prov.OSMID]; len(got) != 1 || got[0] != reg {
		t.Errorf("Riau's children = %v, want Kampar", got)
	}
	if got := tree.Children[reg.OSMID]; len(got) != 1 || got[0] != dist {
		t.Errorf("Kampar's children = %v, want its district", got)
	}
	if want := []int64{prov.OSMID, reg.OSMID}; !equal(dist.Ancestors, want) {
		t.Errorf("district ancestors = %v, want %v", dist.Ancestors, want)
	}
	if len(tree.Issues) != 0 {
		t.Errorf("unexpected issues: %+v", tree.Issues)
	}
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
