package emit

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestSitePageLinksPointAtFilesTheDatasetHas keeps site/index.html, the GitHub
// Pages landing page, honest: every v2/... path it mentions must exist in what
// Write produces, so a change of layout cannot leave dead links behind.
func TestSitePageLinksPointAtFilesTheDatasetHas(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "..", "site", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := Write(sampleTree(), Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}

	// Paths in the page start with v2/ and name a file. Placeholders such as
	// <province id> are not links, and the page's own prose mentions are
	// matched too on purpose.
	refs := regexp.MustCompile(`v2/[A-Za-z0-9_./-]+\.(?:json|geojson)`).FindAllString(string(page), -1)
	if len(refs) < 4 {
		t.Fatalf("found only %d dataset links in the page, the test is not reading it properly", len(refs))
	}
	// report.json is written by `toi build` after Write, not by Write.
	producedElsewhere := map[string]bool{"v2/report.json": true}
	for _, ref := range refs {
		if producedElsewhere[ref] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ref[len("v2/"):])); err != nil {
			t.Errorf("site/index.html mentions %s, which the dataset does not contain", ref)
		}
	}
}

// TestSitePageUsesNoExternalScripts: the landing page must stay self-contained.
func TestSitePageUsesNoExternalScripts(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("..", "..", "site", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?i)<script[^>]+src=|<link[^>]+rel=["']?stylesheet`).Match(page) {
		t.Error("site/index.html loads an external script or stylesheet")
	}
}
