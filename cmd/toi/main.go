// Command toi builds the Territory of Indonesia dataset from OpenStreetMap.
//
//	toi fetch      download a Geofabrik extract
//	toi extract    assemble administrative boundary polygons with osmium
//	toi build      build the hierarchy and write the dataset
//	toi validate   check a written dataset
//	toi all        all of the above
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(stderr, nil))

	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "fetch":
		_, err = cmdFetch(ctx, log, rest, stdout, stderr)
	case "extract":
		err = cmdExtract(ctx, log, rest, stderr)
	case "build":
		err = cmdBuild(log, rest, stdout, stderr)
	case "validate":
		err = cmdValidate(rest, stdout, stderr)
	case "all":
		err = cmdAll(ctx, log, rest, stdout, stderr)
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, toolVersion())
	case "help", "-h", "--help":
		usage(stdout)
	default:
		fmt.Fprintf(stderr, "toi: unknown command %q\n\n", cmd)
		usage(stderr)
		return 2
	}

	var ue usageError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.As(err, &ue):
		fmt.Fprintln(stderr, "toi:", ue.msg)
		return 2
	default:
		fmt.Fprintln(stderr, "toi:", err)
		return 1
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `toi builds the Territory of Indonesia dataset from OpenStreetMap.

Usage: toi <command> [flags]

Commands:
  fetch      download a dated Geofabrik extract and verify its checksum
  extract    assemble administrative boundary polygons from the extract (needs osmium or Docker)
  build      resolve the hierarchy and write the dataset and a build report
  validate   check a written dataset; exits 1 on errors
  all        fetch, extract, build and validate in one go
  version    print the version

Run "toi <command> -h" for the flags of a command.
`)
}

func toolVersion() string {
	v := "toi " + version
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				v += " (" + s.Value[:7] + ")"
			}
		}
	}
	return v
}

// flagSet returns a FlagSet that reports problems instead of exiting.
func flagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("toi "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{msg: "invalid flags"}
	}
	if fs.NArg() > 0 {
		return usageError{msg: fmt.Sprintf("unexpected argument %q", fs.Arg(0))}
	}
	return nil
}

func abs(p string) (string, error) { return filepath.Abs(p) }
