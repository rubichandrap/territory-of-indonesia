package emit

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestOutputMatchesTheJSONSchema is the contract test: whatever Write emits
// must validate against schema/territory.schema.json, so the schema (and the
// types generated from it) cannot drift away from the real output.
func TestOutputMatchesTheJSONSchema(t *testing.T) {
	c := jsonschema.NewCompiler()
	const base = "https://github.com/rubichandrap/territory-of-indonesia/schema/territory.schema.json"
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "territory.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddResource(base, doc); err != nil {
		t.Fatal(err)
	}
	compile := func(def string) *jsonschema.Schema {
		s, err := c.Compile(base + "#/$defs/" + def)
		if err != nil {
			t.Fatalf("compiling %s: %v", def, err)
		}
		return s
	}
	manifest, entry, collection := compile("Manifest"), compile("IndexEntry"), compile("FeatureCollection")

	dir := t.TempDir()
	src := Source{
		Name: "OpenStreetMap", Provider: "Geofabrik", Timestamp: "2026-11-14T23:59:59Z",
		Extracts: []Extract{{
			File: "indonesia-261114.osm.pbf", URL: "https://download.geofabrik.de/asia/indonesia-261114.osm.pbf",
			SHA256: strings.Repeat("a", 64), Timestamp: "2026-11-14T23:59:59Z",
		}},
	}
	if _, err := Write(sampleTree(), Options{Dir: dir, Source: src, Tool: "toi test"}); err != nil {
		t.Fatal(err)
	}

	validated := 0
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			return nil
		}
		switch {
		case rel == "manifest.json":
			check(t, manifest, rel, inst)
		case strings.HasPrefix(rel, "index/"):
			arr, ok := inst.([]any)
			if !ok || len(arr) == 0 {
				t.Errorf("%s: expected a non-empty array", rel)
				return nil
			}
			for i, e := range arr {
				check(t, entry, rel+"["+string(rune('0'+i))+"]", e)
			}
		default:
			check(t, collection, rel, inst)
		}
		validated++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if validated < 10 {
		t.Errorf("validated only %d files", validated)
	}

	// The schema must reject what it is supposed to reject.
	var bad any
	_ = json.Unmarshal([]byte(`{"osm_id":1,"level":"province","admin_level":4,"name":"x","bbox":[1,2,3,4],"center":[1,2],"surprise":1}`), &bad)
	if err := entry.Validate(bad); err == nil {
		t.Error("an index entry with an unknown property was accepted")
	}
}

func check(t *testing.T, s *jsonschema.Schema, name string, inst any) {
	t.Helper()
	if err := s.Validate(inst); err != nil {
		t.Errorf("%s does not match the schema:\n%v", name, err)
	}
}
