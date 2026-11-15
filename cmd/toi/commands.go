package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rubichandrap/territory-of-indonesia/internal/emit"
	"github.com/rubichandrap/territory-of-indonesia/internal/fetch"
	"github.com/rubichandrap/territory-of-indonesia/internal/hierarchy"
	"github.com/rubichandrap/territory-of-indonesia/internal/model"
	"github.com/rubichandrap/territory-of-indonesia/internal/osmium"
	"github.com/rubichandrap/territory-of-indonesia/internal/report"
	"github.com/rubichandrap/territory-of-indonesia/internal/source"
	"github.com/rubichandrap/territory-of-indonesia/internal/validate"
)

const (
	defaultCache = ".cache"
	defaultOut   = "dist"
)

func cmdFetch(ctx context.Context, log *slog.Logger, args []string, stdout, stderr io.Writer) (*fetch.Result, error) {
	fs := flagSet("fetch", stderr)
	region := fs.String("region", "asia/indonesia", "Geofabrik region path")
	snapshot := fs.String("snapshot", "latest", `"latest" or a dated snapshot such as 261114`)
	cache := fs.String("cache", defaultCache, "cache directory; the file goes into <cache>/geofabrik")
	if err := parse(fs, args); err != nil {
		return nil, err
	}
	res, err := fetch.Get(ctx, fetch.Options{
		Region: *region, Snapshot: *snapshot, Dir: filepath.Join(*cache, "geofabrik"),
		UserAgent: "toi/" + version,
	}, log)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(stdout, res.Path)
	return res, nil
}

func workFlag(fs *flag.FlagSet) *string {
	return fs.String("work", filepath.Join(defaultCache, "work"), "directory for intermediate files")
}

func addRunnerFlags(fs *flag.FlagSet) (mode, image *string) {
	mode = fs.String("osmium", "auto", "how to run osmium: auto, local or docker")
	image = fs.String("image", osmium.DefaultImage, "docker image to use (built from the embedded Dockerfile if missing)")
	return
}

// stringList is a flag that can be given several times. Giving it at all
// replaces the default value.
type stringList struct {
	vals []string
	set  bool
}

func (l *stringList) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(l.vals, ",")
}

func (l *stringList) Set(v string) error {
	if !l.set {
		l.vals, l.set = nil, true
	}
	l.vals = append(l.vals, v)
	return nil
}

func cmdExtract(ctx context.Context, log *slog.Logger, args []string, stderr io.Writer) error {
	fs := flagSet("extract", stderr)
	pbfs := &stringList{}
	fs.Var(pbfs, "pbf", "an .osm.pbf to read; repeat it to add the neighbouring country's extract (required)")
	work := workFlag(fs)
	mode, image := addRunnerFlags(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	if len(pbfs.vals) == 0 {
		return usageError{msg: "extract: -pbf is required"}
	}
	return extract(ctx, log, pbfs.vals, *work, osmium.Mode(*mode), *image, stderr)
}

func extract(ctx context.Context, log *slog.Logger, pbfs []string, work string, mode osmium.Mode, image string, stderr io.Writer) error {
	work, err := abs(work)
	if err != nil {
		return err
	}
	var (
		inputs []osmium.Input
		paths  []string
	)
	for _, p := range pbfs {
		if p, err = abs(p); err != nil {
			return err
		}
		if _, err := os.Stat(p); err != nil {
			return err
		}
		in := osmium.Input{Path: p}
		if si, ok := fetch.ReadSidecar(p); ok {
			in.URL = si.URL
		}
		inputs = append(inputs, in)
		paths = append(paths, p)
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	runner, err := osmium.Detect(ctx, mode, image, osmium.Mounts(paths, work), stderr)
	if err != nil {
		return err
	}
	start := time.Now()
	if _, err := osmium.Extract(ctx, runner, inputs, work, log); err != nil {
		return err
	}
	log.Info("extraction finished", "took", time.Since(start).Round(time.Second), "work", work)
	return nil
}

func addOutputFlags(fs *flag.FlagSet) (out, baseline *string, maxDrop *float64) {
	out = fs.String("out", defaultOut, "output directory")
	baseline = fs.String("baseline", "", "manifest.json of a previous accepted build; a drop in counts fails validation")
	maxDrop = fs.Float64("max-drop", 0.02, "largest tolerated drop in region counts against the baseline, as a fraction")
	return
}

func cmdBuild(log *slog.Logger, args []string, stdout, stderr io.Writer) error {
	fs := flagSet("build", stderr)
	work := workFlag(fs)
	out, _, _ := addOutputFlags(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	return build(log, *work, *out, stdout)
}

func build(log *slog.Logger, work, out string, stdout io.Writer) error {
	meta, err := osmium.ReadMeta(work)
	if err != nil {
		return fmt.Errorf("no extraction found in %s (run toi extract first): %w", work, err)
	}

	f, err := os.Open(filepath.Join(work, osmium.RegionsFile))
	if err != nil {
		return err
	}
	regions, readStats, err := source.Read(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("reading %s: %w", osmium.RegionsFile, err)
	}
	log.Info("read boundaries", "polygons", len(regions), "skipped", readStats.Skipped)

	var expected []osmium.Relation
	if rf, err := os.Open(filepath.Join(work, osmium.RelationsFile)); err == nil {
		expected, err = osmium.ParseRelations(rf)
		rf.Close()
		if err != nil {
			return fmt.Errorf("reading %s: %w", osmium.RelationsFile, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	start := time.Now()
	tree := hierarchy.Build(regions)
	log.Info("hierarchy resolved", "kept", len(tree.Regions), "issues", len(tree.Issues), "took", time.Since(start).Round(time.Millisecond))
	if len(tree.Children[0]) == 0 {
		return errors.New("no Indonesian province found in the extraction; refusing to write an empty dataset")
	}

	src := emit.Source{Name: "OpenStreetMap", Provider: "Geofabrik", Timestamp: meta.OldestTimestamp()}
	for _, in := range meta.Inputs {
		src.Extracts = append(src.Extracts, emit.Extract{File: in.File, URL: in.URL, SHA256: in.SHA256, Timestamp: in.OSMTimestamp})
	}
	start = time.Now()
	m, err := emit.Write(tree, emit.Options{Dir: out, Source: src, Tool: toolVersion()})
	if err != nil {
		return err
	}
	log.Info("dataset written", "dir", out, "took", time.Since(start).Round(time.Millisecond))

	rep := report.Build(report.Input{
		Source: src, Skipped: readStats.Skipped, Expected: expected,
		Assembled: regions, Tree: tree, ExportWarnings: meta.ExportWarnings,
	})
	if err := rep.WriteJSON(filepath.Join(out, "report.json")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "report.md"), []byte(rep.Markdown()), 0o644); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "wrote %s\n", out)
	for _, l := range model.Levels() {
		fmt.Fprintf(stdout, "  %-10s %5d\n", l.Plural(), m.Counts[l.Plural()])
	}
	for _, t := range []string{emit.TierIndex, emit.TierSimplified, emit.TierFull} {
		fmt.Fprintf(stdout, "  %-10s %5d files %8.1f MB\n", t, m.Tiers[t].Files, float64(m.Tiers[t].Bytes)/1e6)
	}
	return nil
}

func cmdValidate(args []string, stdout, stderr io.Writer) error {
	fs := flagSet("validate", stderr)
	dist := fs.String("dist", defaultOut, "dataset directory")
	baseline := fs.String("baseline", "", "manifest.json of a previous accepted build")
	maxDrop := fs.Float64("max-drop", 0.02, "largest tolerated drop in region counts against the baseline, as a fraction")
	if err := parse(fs, args); err != nil {
		return err
	}
	return runValidate(*dist, *baseline, *maxDrop, stdout)
}

func runValidate(dist, baseline string, maxDrop float64, stdout io.Writer) error {
	opt := validate.Options{Dir: dist, MaxDrop: maxDrop}
	if baseline != "" {
		bl, err := validate.LoadBaseline(baseline)
		if err != nil {
			return err
		}
		opt.Baseline = bl
	}
	res, err := validate.Run(opt)
	if err != nil {
		return err
	}
	const show = 40
	for i, f := range res.Findings {
		if i == show {
			fmt.Fprintf(stdout, "... and %d more findings\n", len(res.Findings)-show)
			break
		}
		fmt.Fprintf(stdout, "%-7s %-18s %s %s\n", f.Severity, f.Check, f.Path, f.Message)
	}
	fmt.Fprintf(stdout, "validated %s: %d regions, %d errors, %d warnings\n",
		dist, total(res.Counts), res.Errors(), len(res.Findings)-res.Errors())
	if n := res.Errors(); n > 0 {
		return fmt.Errorf("validation failed with %d errors", n)
	}
	return nil
}

func total(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func cmdAll(ctx context.Context, log *slog.Logger, args []string, stdout, stderr io.Writer) error {
	fs := flagSet("all", stderr)
	region := fs.String("region", "asia/indonesia", "Geofabrik region path")
	neighbors := &stringList{vals: []string{"asia/malaysia-singapore-brunei"}}
	fs.Var(neighbors, "neighbor", `a neighbouring region whose extract completes boundaries on the border, repeatable; "none" for no neighbours`)
	snapshot := fs.String("snapshot", "latest", `"latest" or a dated snapshot such as 261114`)
	cache := fs.String("cache", defaultCache, "cache directory")
	work := workFlag(fs)
	mode, image := addRunnerFlags(fs)
	out, baseline, maxDrop := addOutputFlags(fs)
	if err := parse(fs, args); err != nil {
		return err
	}

	var pbfs []string
	for _, r := range append([]string{*region}, neighborsOf(neighbors)...) {
		res, err := fetch.Get(ctx, fetch.Options{
			Region: r, Snapshot: *snapshot, Dir: filepath.Join(*cache, "geofabrik"), UserAgent: "toi/" + version,
		}, log)
		if err != nil {
			return err
		}
		pbfs = append(pbfs, res.Path)
	}
	if err := extract(ctx, log, pbfs, *work, osmium.Mode(*mode), *image, stderr); err != nil {
		return err
	}
	if err := build(log, *work, *out, stdout); err != nil {
		return err
	}
	return runValidate(*out, *baseline, *maxDrop, stdout)
}

// neighborsOf resolves the -neighbor flag; "none" means no neighbours.
func neighborsOf(l *stringList) []string {
	var out []string
	for _, n := range l.vals {
		if n != "none" && n != "" {
			out = append(out, n)
		}
	}
	return out
}
