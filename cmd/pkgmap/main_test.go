package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestClassificationIsComplete is `go run ./cmd/pkgmap -check` against this
// checkout: every package has a tier and status in packages.tiers.json, and
// every classified path still exists. Adding, moving or deleting a package
// means editing that file in the same commit.
func TestClassificationIsComplete(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	m, err := load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.pkgs) < 50 {
		t.Fatalf("found only %d packages; the walk is broken", len(m.pkgs))
	}
	for _, p := range m.problems() {
		t.Error(p)
	}
	if t.Failed() {
		t.Logf("edit %s (tier + status per package)", classFile)
	}
}

// TestRenderIsDeterministic renders twice and checks the fixed sections.
func TestRenderIsDeterministic(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	for _, w := range []*bytes.Buffer{&a, &b} {
		m, err := load(root)
		if err != nil {
			t.Fatal(err)
		}
		m.render(w)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("two renders differ")
	}
	for _, want := range []string{"| `rules` |", "| `cmd/pkgmap` |", "```mermaid", "## Tier dependencies"} {
		if !strings.Contains(a.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
