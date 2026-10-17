package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// allocMiB is what the fixture test holds live and touched, so its test
// binary's peak RSS must be at least this much.
const allocMiB = 64

// testbudgetBinary builds cmd/testbudget into a per-test temp directory (the
// build cache makes the second build in one run nearly free).
func testbudgetBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "testbudget")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build cmd/testbudget: %v\n%s", err, out)
	}
	return bin
}

// fixtureModule writes a throwaway module whose package alloc has one test
// that allocates and touches allocMiB. The source embeds the fresh temp
// directory's path (and logs it, so it survives into the binary, which Go
// also caches by), so no earlier run's cached result can serve its first run.
func fixtureModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":         "module example.com/budgetfixture\n\ngo 1.22\n",
		"alloc/alloc.go": "package alloc\n",
		"alloc/alloc_test.go": fmt.Sprintf(`package alloc

import "testing"

const nonce = %q

var sink []byte

func TestAlloc(t *testing.T) {
	t.Log(nonce) // keeps the nonce in the binary: Go also caches by binary content
	sink = make([]byte, %d<<20)
	for i := range sink {
		sink[i] = byte(i)
	}
}
`, root, allocMiB),
	}
	for name, body := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// goTestExec runs `go test -exec "<testbudget> exec" ./alloc` in root.
func goTestExec(t *testing.T, root, bin string, env ...string) string {
	t.Helper()
	cmd := exec.Command("go", "test", "-exec", bin+" exec", "./alloc")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), append([]string{"GOFLAGS=", "GOWORK=off"}, env...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test -exec: %v\n%s", err, out)
	}
	return string(out)
}

// TestExecKeepsGoTestCacheUsable guards the -test.testlogfile workaround in
// runExec: the post-merge full suite runs the whole suite under -exec, and without
// the workaround every package re-runs on every batch. If a future Go breaks
// it, this fails so the slowdown is noticed rather than silently paid.
func TestExecKeepsGoTestCacheUsable(t *testing.T) {
	bin := testbudgetBinary(t)
	root := fixtureModule(t)
	if out := goTestExec(t, root, bin); strings.Contains(out, "(cached)") {
		t.Fatalf("first run of a fresh fixture was cached; the guard proves nothing:\n%s", out)
	}
	if out := goTestExec(t, root, bin); !strings.Contains(out, "(cached)") {
		t.Fatalf("second identical `go test -exec \"testbudget exec\"` was not cached: "+
			"runExec's -test.testlogfile workaround no longer works on this Go, so every "+
			"post-merge package re-runs every batch:\n%s", out)
	}
}

// TestExecRecordsFixturePeakRSS runs a real test binary through `go test
// -exec` and checks the recorded peak covers what the test allocated, under
// the package's module-relative key.
func TestExecRecordsFixturePeakRSS(t *testing.T) {
	bin := testbudgetBinary(t)
	root := fixtureModule(t)
	rss := t.TempDir()
	goTestExec(t, root, bin, rssDirEnv+"="+rss)
	peaks, err := readRSSDir(rss)
	if err != nil {
		t.Fatal(err)
	}
	if kib := peaks["alloc"]; kib < allocMiB*1024 {
		t.Fatalf("recorded peak RSS for alloc = %d KiB, want >= %d KiB (the test touched %d MiB): %v",
			kib, allocMiB*1024, allocMiB, peaks)
	}
}

func TestWithTestlogAddsTheFlagOnlyWhenMissing(t *testing.T) {
	got := withTestlog([]string{"/w/b001/p.test", "-test.v"})
	want := []string{"/w/b001/p.test", "-test.testlogfile=/w/b001/testlog.txt", "-test.v"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("withTestlog = %q, want %q", got, want)
	}
	for _, given := range [][]string{
		{"/w/p.test", "-test.testlogfile=/x"},
		{"/w/p.test", "--test.testlogfile=/x"},
		{"/w/p.test", "-test.testlogfile", "/x"},
		{"/bin/sh", "-c", "exit 0"},
	} {
		if got := withTestlog(given); strings.Join(got, " ") != strings.Join(given, " ") {
			t.Errorf("withTestlog(%q) = %q, want it unchanged", given, got)
		}
	}
}
