package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// layering_engine_tests_test.go keeps the compliance tier out of the engine
// packages' TEST binaries. Production engine code already imports nothing
// above the engine (TestDependencyOrderHolds); this pins the test side,
// which the other layering checks deliberately exclude.
//
// Why it matters (repo-shape audit, 2026-10-14): two rules_test files that
// imported lab/oracle/compliance/oraclegen/templates and lab/oracle/compliance/levelb put the
// whole compliance tier in ./engine/rules' test binary. The templates are the most
// edited Go code in the repo, so 63% of the 883 merges 10-05..10-14
// invalidated ./engine/rules' test cache (193 s of tests and a 27 s rebuild per
// gate run) against 42% once the edge was gone. A test that drives a
// compliance scenario through the engine belongs in the compliance package
// (lab/oracle/compliance/oraclegen/templates/rules_oracle_*_test.go are the two that
// moved), which depends on rules anyway.
//
// The check parses the _test.go files itself instead of asking `go list`:
// go test's result cache tracks only files the test process opens, so a
// subprocess read would let a cached pass hide a newly added violation.

// engineTestRoots are the engine directories (walked recursively) whose test
// files may not import compliance.
var engineTestRoots = []string{"rules", "effects", "state", "events", "cards", "decision", "deck", "replay"}

func TestEngineTestsDoNotImportCompliance(t *testing.T) {
	root := filepath.Join("..", "..")
	var bad []string
	files := 0
	fset := token.NewFileSet()
	for _, r := range engineTestRoots {
		err := filepath.WalkDir(filepath.Join(root, r), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			files++
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range f.Imports {
				imp, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				if imp == module+"/compliance" || strings.HasPrefix(imp, module+"/compliance/") {
					rel, _ := filepath.Rel(root, path)
					bad = append(bad, rel+" imports "+imp)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", r, err)
		}
	}
	if files == 0 {
		t.Fatal("found no engine test files; the walk root is wrong")
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		t.Fatalf("engine test files import the compliance tier; move the test into the compliance package it exercises:\n  %s",
			strings.Join(bad, "\n  "))
	}
}
