package emit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// TestTypeScriptTypesCoverTheSchema fails when schema/territory.schema.json
// has a property that types/territory.d.ts does not declare. It is a drift
// guard, not a type checker: it only looks for the property names.
func TestTypeScriptTypesCoverTheSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "territory.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	dts, err := os.ReadFile(filepath.Join("..", "..", "types", "territory.d.ts"))
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{}
	collectProperties(schema, want)
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) < 25 {
		t.Fatalf("found only %d property names in the schema, the test is not reading it properly", len(names))
	}
	for _, n := range names {
		if !regexp.MustCompile(`(^|\s|[{;])` + regexp.QuoteMeta(n) + `\??:`).Match(dts) {
			t.Errorf("types/territory.d.ts does not declare %q", n)
		}
	}
}

// collectProperties adds the name of every property declared anywhere in a
// JSON Schema document, at any depth.
func collectProperties(node any, out map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if props, ok := child.(map[string]any); ok && key == "properties" {
				for name, sub := range props {
					out[name] = true
					collectProperties(sub, out)
				}
				continue
			}
			collectProperties(child, out)
		}
	case []any:
		for _, child := range v {
			collectProperties(child, out)
		}
	}
}
