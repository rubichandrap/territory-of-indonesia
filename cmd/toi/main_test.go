package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestStringListGivenFlagReplacesTheDefault(t *testing.T) {
	l := &stringList{vals: []string{"default"}}
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.Var(l, "n", "")

	if err := fs.Parse(nil); err != nil || strings.Join(neighborsOf(l), ",") != "default" {
		t.Fatalf("without the flag the default must stay, got %v", neighborsOf(l))
	}
	if err := fs.Parse([]string{"-n", "a", "-n", "b"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(neighborsOf(l), ","); got != "a,b" {
		t.Errorf("neighbours = %q, want a,b (the default must be replaced, not extended)", got)
	}
}

func TestNeighborNoneMeansNoNeighbours(t *testing.T) {
	l := &stringList{vals: []string{"default"}}
	if err := l.Set("none"); err != nil {
		t.Fatal(err)
	}
	if got := neighborsOf(l); len(got) != 0 {
		t.Errorf("neighbours = %v, want none", got)
	}
}

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no arguments", nil, 2},
		{"unknown command", []string{"frobnicate"}, 2},
		{"help", []string{"help"}, 0},
		{"version", []string{"version"}, 0},
		{"command help", []string{"build", "-h"}, 0},
		{"bad flag", []string{"build", "-nope"}, 2},
		{"stray argument", []string{"build", "extra"}, 2},
		{"extract needs input", []string{"extract"}, 2},
		{"validate a missing directory", []string{"validate", "-dist", t.TempDir() + "/missing"}, 1},
	}
	for _, tt := range tests {
		var out, errb bytes.Buffer
		if got := run(tt.args, &out, &errb); got != tt.want {
			t.Errorf("%s: exit code %d, want %d (stderr: %s)", tt.name, got, tt.want, errb.String())
		}
	}
}
