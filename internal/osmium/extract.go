package osmium

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Files the extraction leaves in the work directory.
const (
	RegionsFile   = "regions.geojsonseq" // one feature per line: the assembled polygons
	RelationsFile = "relations.opl"      // the matching relations, for counting and cross-checks
	MetaFile      = "extract.json"
	ExportLogFile = "export.log" // osmium's complaints about relations it could not assemble
)

// Input is one OSM extract to read.
type Input struct {
	Path string // absolute
	URL  string // where it came from; only recorded
}

// InputMeta records one input of an extraction.
type InputMeta struct {
	File         string `json:"file"`
	URL          string `json:"url,omitempty"`
	SHA256       string `json:"sha256"`
	OSMTimestamp string `json:"osm_timestamp,omitempty"` // replication timestamp of the data in the PBF
}

// Meta records where an extraction came from.
type Meta struct {
	Inputs         []InputMeta `json:"inputs"`
	OsmiumVersion  string      `json:"osmium_version"`
	Runner         string      `json:"runner"`
	ExportWarnings int         `json:"export_log_lines"`
}

// OldestTimestamp returns the earliest OSM timestamp of the inputs: the data
// is at least that current. It is "" when no input has one.
func (m *Meta) OldestTimestamp() string {
	var (
		oldest time.Time
		out    string
	)
	for _, in := range m.Inputs {
		t, err := time.Parse(time.RFC3339, in.OSMTimestamp)
		if err != nil {
			continue
		}
		if out == "" || t.Before(oldest) {
			oldest, out = t, in.OSMTimestamp
		}
	}
	return out
}

// Extract turns OSM extracts into administrative boundary polygons.
//
//  1. keep only boundary=administrative relations (and what they reference),
//     separately in every input,
//  2. merge the results, so a boundary that runs along a border can be
//     completed from the neighbouring country's extract; an extract is cut at
//     the border and the ways on the cut are missing from it,
//  3. of those keep admin_level 4-7,
//  4. let osmium assemble the member ways into polygons (the hard part: rings
//     must be stitched from many ways, and osmium's assembler is the reference
//     implementation),
//  5. dump the relations as OPL, so the build can tell how many relations
//     exist against how many assembled.
//
// Inputs must have absolute paths, and so must work, which is created if needed.
func Extract(ctx context.Context, r Runner, inputs []Input, work string, log *slog.Logger) (*Meta, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("osmium: no input")
	}
	for _, in := range inputs {
		if !filepath.IsAbs(in.Path) {
			return nil, fmt.Errorf("osmium: input %s must be an absolute path", in.Path)
		}
	}
	if !filepath.IsAbs(work) {
		return nil, fmt.Errorf("osmium: work must be an absolute path")
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, err
	}
	p := func(name string) string { return filepath.Join(work, name) }
	meta := &Meta{Runner: r.Describe()}

	v, err := capture(ctx, r, "--version")
	if err != nil {
		return nil, err
	}
	meta.OsmiumVersion, _, _ = strings.Cut(v, "\n")
	log.Info("osmium", "version", meta.OsmiumVersion, "runner", meta.Runner)

	var filtered []string
	for i, in := range inputs {
		im := InputMeta{File: filepath.Base(in.Path), URL: in.URL}
		info, err := capture(ctx, r, "fileinfo", "-j", in.Path)
		if err != nil {
			return nil, err
		}
		if im.OSMTimestamp, err = timestampFromFileinfo(info); err != nil {
			log.Warn("no replication timestamp in the PBF header", "file", im.File, "err", err)
		}
		if im.SHA256, err = fileSHA256(in.Path); err != nil {
			return nil, err
		}
		meta.Inputs = append(meta.Inputs, im)

		out := p("admin-" + strconv.Itoa(i) + ".osm.pbf")
		log.Info("osmium", "step", "filter boundary=administrative", "input", im.File)
		if _, err := capture(ctx, r, "tags-filter", in.Path, "r/boundary=administrative", "-o", out, "--overwrite"); err != nil {
			return nil, err
		}
		filtered = append(filtered, out)
	}

	admin := filtered[0]
	if len(filtered) > 1 {
		admin = p("admin.osm.pbf")
		log.Info("osmium", "step", "merge inputs", "count", len(filtered))
		args := append([]string{"merge"}, filtered...)
		args = append(args, "-o", admin, "--overwrite")
		if _, err := capture(ctx, r, args...); err != nil {
			return nil, err
		}
	}

	levels := p("levels.osm.pbf")
	steps := []struct {
		name string
		args []string
	}{
		{"filter admin_level 4-7", []string{"tags-filter", admin, "r/admin_level=4,5,6,7", "-o", levels, "--overwrite"}},
		{"dump relations", []string{"cat", levels, "-t", "relation", "-f", "opl", "-o", p(RelationsFile), "--overwrite"}},
	}
	for _, s := range steps {
		log.Info("osmium", "step", s.name)
		if _, err := capture(ctx, r, s.args...); err != nil {
			return nil, err
		}
	}

	log.Info("osmium", "step", "assemble polygons")
	logf, err := os.Create(p(ExportLogFile))
	if err != nil {
		return nil, err
	}
	defer logf.Close()
	if err := r.Run(ctx, io.Discard, logf,
		"export", levels,
		"-f", "geojsonseq", "-x", "print_record_separator=false",
		"--geometry-types=polygon", "--add-unique-id=type_id", "--show-errors",
		"-o", p(RegionsFile), "--overwrite",
	); err != nil {
		return nil, fmt.Errorf("osmium export: %w (see %s)", err, p(ExportLogFile))
	}
	meta.ExportWarnings = countLines(p(ExportLogFile))

	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	return meta, os.WriteFile(p(MetaFile), append(b, '\n'), 0o644)
}

// ReadMeta loads the extract.json of a work directory.
func ReadMeta(work string) (*Meta, error) {
	b, err := os.ReadFile(filepath.Join(work, MetaFile))
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if len(m.Inputs) == 0 {
		return nil, fmt.Errorf("%s lists no inputs; it was written by an older version, run toi extract again", MetaFile)
	}
	return &m, nil
}

// timestampFromFileinfo reads the data timestamp from `osmium fileinfo -j`.
func timestampFromFileinfo(js string) (string, error) {
	var info struct {
		Header struct {
			Option map[string]string `json:"option"`
		} `json:"header"`
	}
	if err := json.Unmarshal([]byte(js), &info); err != nil {
		return "", err
	}
	for _, k := range []string{"osmosis_replication_timestamp", "timestamp"} {
		if v := info.Header.Option[k]; v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("header has neither osmosis_replication_timestamp nor timestamp")
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		n++
	}
	return n
}
