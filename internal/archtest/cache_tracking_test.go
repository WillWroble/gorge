package archtest

import "testing"

// TestOpenPackageDirsCoversEveryPackage guards the cache-correctness helper
// introduced for cli-20261014T200058Z-c466ec4b. The go-list-based checks read
// the module through a `go list` subprocess, whose reads go test's result
// cache cannot see; openPackageDirs opens every package directory from the
// test process so the cache tracks the source (see packages' doc comment).
//
// The failure modes this test pins are exactly the ones that would silently
// reopen the stale-cache hole:
//
//   - an empty package map (the open would run vacuously);
//   - a package with no directory (the open would be skipped for it);
//   - a directory that cannot be opened (the open silently failed).
//
// It must be able to fail: the empty-map and dir-less cases assert the helper
// rejects them, and the live case asserts it accepts the real module. A
// regression that drops either guard fails here rather than being served a
// stale cached pass.
func TestOpenPackageDirsCoversEveryPackage(t *testing.T) {
	pkgs := packages(t)
	if len(pkgs) < 10 {
		t.Fatalf("packages() returned only %d packages; the guard would run vacuously", len(pkgs))
	}
	opened, err := openPackageDirs(pkgs)
	if err != nil {
		t.Errorf("openPackageDirs(%d real packages) = _, %v, want nil", len(pkgs), err)
	}
	if opened != len(pkgs) {
		t.Errorf("openPackageDirs opened %d directories for %d packages; a skipped open leaves the stale-cache hole open", opened, len(pkgs))
	}
	if _, err := openPackageDirs(map[string]pkg{}); err == nil {
		t.Error("openPackageDirs accepted an empty package map; the cache-tracking open would run vacuously")
	}
	if _, err := openPackageDirs(map[string]pkg{"x": {path: "x"}}); err == nil {
		t.Error("openPackageDirs accepted a package with no directory; the cache-tracking open would be skipped for it")
	}
}
