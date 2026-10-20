package main

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

var update = flag.Bool("update", false, "rewrite testdata/heads.txt")

// goldenPairs are the decks the mage fork's configs pit against each other:
// Mage.MageZero/config/config.yml and Mage.Tests/config/config.yml.
var goldenPairs = [][]string{
	{"pauper-faeries", "pauper-cawgates"},
	{"uwtempo", "standard-monob"},
}

// TestGoldenHeads is mz-gorge's regression baseline: fixed seeds, both tap
// modes, byte-identical game lines. A change that moves a line changed how
// gorge plays; run with -update only when that change is intended.
func TestGoldenHeads(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	var buf bytes.Buffer
	for _, p := range goldenPairs {
		if err := run(&buf, reg, p, 8, 1, "both", 0); err != nil {
			t.Fatal(err)
		}
	}
	const path = "testdata/heads.txt"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(buf.String(), "\n")
	exp := strings.Split(string(want), "\n")
	if len(got) != len(exp) {
		t.Fatalf("got %d lines, want %d", len(got), len(exp))
	}
	for i := range got {
		if got[i] != exp[i] {
			t.Errorf("line %d\n got: %s\nwant: %s", i+1, got[i], exp[i])
		}
	}
}
