package osmium

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRelationsReadsTags(t *testing.T) {
	opl := strings.Join([]string{
		"n1 v1 x1 y2", // not a relation
		"r304751 v9 dV c1 t2020-01-01T00:00:00Z i1 uBob Tadmin_level=2,boundary=administrative,name=Indonesia Mw1@outer,r300@subarea",
		"r42 v3 dV c1 t2020-01-01T00:00:00Z i1 uBob Tadmin_level=7,boundary=administrative,name=Kampung%20%Baru%2c%Utara,official_name=Desa%20%Jati%e9% Mw5@outer",
		"r43 v1 T M",
	}, "\n")
	// The first relation above deliberately has a raw space in a tag value,
	// which real OPL never contains; the parser must not choke on it.
	rels, err := ParseRelations(strings.NewReader(opl))
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 3 {
		t.Fatalf("got %d relations, want 3: %+v", len(rels), rels)
	}

	r42 := rels[1]
	if r42.ID != 42 {
		t.Fatalf("second relation id = %d", r42.ID)
	}
	if got := r42.Tags["name"]; got != "Kampung Baru,Utara" {
		t.Errorf("name = %q", got)
	}
	if got := r42.Tags["official_name"]; got != "Desa Jatié" {
		t.Errorf("official_name = %q", got)
	}
	if r42.Tags["admin_level"] != "7" || r42.Tags["boundary"] != "administrative" {
		t.Errorf("tags = %v", r42.Tags)
	}
	if len(rels[2].Tags) != 0 {
		t.Errorf("a relation with empty T and M should have no tags, got %v", rels[2].Tags)
	}
}

func TestParseRelationsSubareaMembers(t *testing.T) {
	line := "r10 v1 Tadmin_level=5 Mr20@subarea,w5@outer,r21@subarea,r22@admin_centre\n"
	rels, err := ParseRelations(strings.NewReader(line))
	if err != nil || len(rels) != 1 {
		t.Fatalf("rels=%v err=%v", rels, err)
	}
	if want := []int64{20, 21}; !reflect.DeepEqual(rels[0].SubareaIDs, want) {
		t.Errorf("subareas = %v, want %v", rels[0].SubareaIDs, want)
	}
}

func TestUnescape(t *testing.T) {
	tests := map[string]string{
		"plain":           "plain",
		"a%20%b":          "a b",
		"a%2c%b":          "a,b",
		"100%25%":         "100%",
		"caf%e9%":         "café",
		"broken%zz%value": "broken%zz%value",
		"dangling%":       "dangling%",
	}
	for in, want := range tests {
		if got := unescape(in); got != want {
			t.Errorf("unescape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTimestampFromFileinfo(t *testing.T) {
	js := `{"header":{"option":{"osmosis_replication_timestamp":"2026-11-14T23:59:59Z","generator":"x"}}}`
	got, err := timestampFromFileinfo(js)
	if err != nil || got != "2026-11-14T23:59:59Z" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := timestampFromFileinfo(`{"header":{"option":{}}}`); err == nil {
		t.Error("expected an error when no timestamp is present")
	}
}

func TestDockerRunArgs(t *testing.T) {
	d := Docker{Image: "img", UID: 1000, GID: 1001, Mounts: Mounts([]string{"/data/in/x.pbf", "/data/in/y.pbf"}, "/data/work")}
	got := strings.Join(d.runArgs([]string{"fileinfo", "/data/in/x.pbf"}), " ")
	want := "run --rm --user 1000:1001 -v /data/in:/data/in:ro -v /data/work:/data/work img fileinfo /data/in/x.pbf"
	if got != want {
		t.Errorf("docker args:\n got %s\nwant %s", got, want)
	}
	if m := Mounts([]string{"/work/x.pbf"}, "/work"); len(m) != 1 {
		t.Errorf("input inside work should need a single mount, got %v", m)
	}
}
