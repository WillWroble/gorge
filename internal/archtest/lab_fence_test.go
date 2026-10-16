package archtest

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// lab is the fenced extension tree (docs/decisions/0001-repo-layout.md):
// draft events, compliance oracles and training research that will move to
// their own repository. The platform -- engine/, server/, bots/, cmd/,
// internal/ -- must build and work without it, so nothing outside lab/ may
// import it. lab/ may import the platform freely.
const lab = module + "/lab"

func inLab(path string) bool { return path == lab || strings.HasPrefix(path, lab+"/") }

// TestNothingOutsideLabImportsLab is the fence for non-test code in the
// default build. A composition point that links lab in on purpose does so
// behind the `lab` build tag (cmd/gorged's events.go), which this listing
// does not see; that is the seam, not a hole.
func TestNothingOutsideLabImportsLab(t *testing.T) {
	packages(t) // result-cache coverage of every package dir (see packages)
	// -tags= pins the untagged build even when the caller exports
	// GOFLAGS=-tags=lab (the gates and the full suite do).
	out, err := exec.Command("go", "list", "-tags=", "-f", `{{.ImportPath}}|{{join .Imports " "}}`, module+"/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var bad []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, imps, _ := strings.Cut(line, "|")
		if inLab(path) {
			continue
		}
		for _, imp := range strings.Fields(imps) {
			if inLab(imp) {
				bad = append(bad, path+" imports "+imp)
			}
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("%s; lab/ is fenced: move the shared code out of lab/, or put the composition behind the lab build tag", b)
	}
}

// allowedLabTestEdges are the platform TEST packages that still import lab/
// (as of the 2026-10-16 restructure): fixtures that borrow the SpellBench
// deck lists. Shrink-only: delete a row when its test stops importing lab,
// never add one.
var allowedLabTestEdges = map[string]bool{
	module + "/bots/azmcts -> " + lab + "/training/spellbench":              true,
	module + "/bots/spellbench/builtins -> " + lab + "/training/spellbench": true,
	module + "/bots/spellbench/sbsearch -> " + lab + "/training/spellbench": true,
	module + "/internal/bench -> " + lab + "/training/spellbench":           true,
}

// TestPlatformTestsImportLabOnlyFromTheAllowlist keeps lab/ out of new
// platform tests: a test that needs lab code lives in lab/.
func TestPlatformTestsImportLabOnlyFromTheAllowlist(t *testing.T) {
	packages(t) // result-cache coverage of every package dir (see packages)
	out, err := exec.Command("go", "list", "-tags=", "-f", `{{.ImportPath}}|{{join .TestImports " "}} {{join .XTestImports " "}}`, module+"/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, imps, _ := strings.Cut(line, "|")
		if inLab(path) {
			continue
		}
		for _, imp := range strings.Fields(imps) {
			if !inLab(imp) {
				continue
			}
			key := path + " -> " + imp
			if allowedLabTestEdges[key] {
				seen[key] = true
				continue
			}
			t.Errorf("%s: a platform test imports lab/; move the test into lab/ or stop importing lab", key)
		}
	}
	for key := range allowedLabTestEdges {
		if !seen[key] {
			t.Errorf("allowedLabTestEdges row %q is stale: the test no longer imports lab; delete the row", key)
		}
	}
}
