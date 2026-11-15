// Package osmium runs the osmium command line tool, either from PATH or from
// a Docker image, and drives the extraction pipeline that turns a Geofabrik
// extract into administrative boundary polygons.
package osmium

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed Dockerfile
var dockerfile []byte

// DefaultImage is the tag used for the image built from the embedded Dockerfile.
const DefaultImage = "toi-osmium:trixie"

// Runner executes osmium. Every path in args must be absolute.
type Runner interface {
	Run(ctx context.Context, stdout, stderr io.Writer, args ...string) error
	// Describe says how osmium is being run, for logs.
	Describe() string
}

// Local runs an osmium binary found on PATH.
type Local struct{ Bin string }

func (l Local) Run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, l.Bin, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func (l Local) Describe() string { return "local binary " + l.Bin }

// Mount is a host directory made visible inside the container at the same
// path, so absolute host paths in the osmium arguments work unchanged.
type Mount struct {
	Path     string
	ReadOnly bool
}

// Docker runs osmium inside a container.
type Docker struct {
	Image  string
	Mounts []Mount
	UID    int
	GID    int
}

func (d Docker) Run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", d.runArgs(args)...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

func (d Docker) Describe() string { return "docker image " + d.Image }

func (d Docker) runArgs(osmiumArgs []string) []string {
	a := []string{"run", "--rm", "--user", strconv.Itoa(d.UID) + ":" + strconv.Itoa(d.GID)}
	for _, m := range d.Mounts {
		v := m.Path + ":" + m.Path
		if m.ReadOnly {
			v += ":ro"
		}
		a = append(a, "-v", v)
	}
	a = append(a, d.Image)
	return append(a, osmiumArgs...)
}

// Mode selects how Detect finds osmium.
type Mode string

const (
	Auto       Mode = "auto"
	ModeLocal  Mode = "local"
	ModeDocker Mode = "docker"
)

// Detect returns a Runner. With Auto it prefers an osmium on PATH and falls
// back to Docker, building the image from the embedded Dockerfile when it is
// not there yet. mounts are only used for Docker.
func Detect(ctx context.Context, mode Mode, image string, mounts []Mount, stderr io.Writer) (Runner, error) {
	if image == "" {
		image = DefaultImage
	}
	if mode == Auto || mode == ModeLocal {
		if bin, err := exec.LookPath("osmium"); err == nil {
			return Local{Bin: bin}, nil
		}
		if mode == ModeLocal {
			return nil, errors.New("osmium not found on PATH (install osmium-tool, or use -osmium=docker)")
		}
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, errors.New("neither osmium nor docker found: install osmium-tool (apt install osmium-tool) or Docker")
	}
	if err := ensureImage(ctx, image, stderr); err != nil {
		return nil, err
	}
	return Docker{Image: image, Mounts: mounts, UID: os.Getuid(), GID: os.Getgid()}, nil
}

func ensureImage(ctx context.Context, image string, stderr io.Writer) error {
	if err := exec.CommandContext(ctx, "docker", "image", "inspect", image).Run(); err == nil {
		return nil
	}
	build := exec.CommandContext(ctx, "docker", "build", "-t", image, "-")
	build.Stdin = bytes.NewReader(dockerfile)
	build.Stdout, build.Stderr = stderr, stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building docker image %s: %w", image, err)
	}
	return nil
}

// Mounts returns what a Docker runner needs to read the inputs (read-only) and
// write to work.
func Mounts(inputs []string, work string) []Mount {
	var mounts []Mount
	seen := map[string]bool{work: true}
	for _, in := range inputs {
		if d := filepath.Dir(in); !seen[d] {
			seen[d] = true
			mounts = append(mounts, Mount{Path: d, ReadOnly: true})
		}
	}
	return append(mounts, Mount{Path: work})
}

// capture runs osmium and returns its stdout, trimmed.
func capture(ctx context.Context, r Runner, args ...string) (string, error) {
	var out, errb bytes.Buffer
	if err := r.Run(ctx, &out, &errb, args...); err != nil {
		return "", fmt.Errorf("osmium %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
