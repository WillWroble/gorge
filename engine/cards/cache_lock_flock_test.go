//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package cards

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// writeColdCorpus writes a tiny synthetic corpus (authored here, not Forge
// text) with no IR cache: every opener's first check misses.
func writeColdCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeScript(t, dir, "mountain.txt", "Name:Mountain\nTypes:Basic Land Mountain\nOracle:\n")
	writeScript(t, dir, "island.txt", "Name:Island\nTypes:Basic Land Island\nOracle:\n")
	return dir
}

// slowCompile is CompileDir held open long enough that every concurrent
// opener reaches the lock while the first is still compiling.
func slowCompile(count func()) func(string) (*Registry, []Diag, error) {
	return func(dir string) (*Registry, []Diag, error) {
		count()
		time.Sleep(150 * time.Millisecond)
		return CompileDir(dir)
	}
}

// TestColdCacheCompilesOnce is the thundering-herd regression: N openers of
// one cold cache compile the corpus exactly once, and every one of them gets
// a registry holding the corpus.
func TestColdCacheCompilesOnce(t *testing.T) {
	dir := writeColdCorpus(t)
	const n = 8
	var compiles atomic.Int32
	compile := slowCompile(func() { compiles.Add(1) })
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	regs := make([]*Registry, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			regs[i], errs[i] = openCorpusWith(dir, "coldfp", compile)
		}()
	}
	close(start)
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("opener %d: %v", i, errs[i])
		}
		if _, ok := regs[i].Lookup("Island"); !ok {
			t.Fatalf("opener %d: registry lacks Island", i)
		}
	}
	if got := compiles.Load(); got != 1 {
		t.Fatalf("%d concurrent openers of a cold cache compiled %d times, want 1", n, got)
	}
}

// coldOpenChildEnv names the corpus dir a re-executed test binary opens in
// TestColdCacheCompilesOnceAcrossProcesses; the child appends one line per
// compile to the file named by coldOpenCountEnv.
const (
	coldOpenChildEnv = "GORGE_CARDS_COLD_OPEN_DIR"
	coldOpenCountEnv = "GORGE_CARDS_COLD_OPEN_COUNT"
)

// TestColdCacheCompilesOnceAcrossProcesses is the same single-flight across
// separate PROCESSES, the real herd shape (one compile per test binary).
func TestColdCacheCompilesOnceAcrossProcesses(t *testing.T) {
	if d := os.Getenv(coldOpenChildEnv); d != "" {
		compile := slowCompile(func() {
			f, err := os.OpenFile(os.Getenv(coldOpenCountEnv), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
			if err != nil {
				t.Fatal(err)
			}
			f.WriteString("compile\n")
			f.Close()
		})
		if _, err := openCorpusWith(d, "coldfp", compile); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := writeColdCorpus(t)
	counter := filepath.Join(t.TempDir(), "count")
	const n = 4
	cmds := make([]*exec.Cmd, n)
	for i := range n {
		cmd := exec.Command(os.Args[0], "-test.run=^TestColdCacheCompilesOnceAcrossProcesses$")
		cmd.Env = append(os.Environ(), coldOpenChildEnv+"="+dir, coldOpenCountEnv+"="+counter)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds[i] = cmd
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("child %d: %v", i, err)
		}
	}
	b, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(b), "compile\n"); got != 1 {
		t.Fatalf("%d processes opening a cold cache compiled %d times, want 1", n, got)
	}
}

// lockHeld reports whether some open file description holds path's lock.
func lockHeld(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := flockRetry(f, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true
	}
	_ = flockRetry(f, syscall.LOCK_UN)
	return false
}

// TestPruneCacheLocksSparesAHeldLock: an orphan lock (its cache gone, nobody
// holding it) is pruned; a held one -- a cold compile in flight -- is not.
func TestPruneCacheLocksSparesAHeldLock(t *testing.T) {
	dir := t.TempDir()
	held := cachePathFor(dir, "held")
	orphan := cachePathFor(dir, "orphan")
	live := cachePathFor(dir, "live")
	unlock := lockCache(held)
	defer unlock()
	lockCache(orphan)()
	lockCache(live)()
	if err := os.WriteFile(live, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pruneCacheLocks(dir)
	if _, err := os.Stat(cacheLockPath(orphan)); !os.IsNotExist(err) {
		t.Errorf("orphan lock survived the prune (err %v)", err)
	}
	if _, err := os.Stat(cacheLockPath(held)); err != nil {
		t.Errorf("held lock was pruned: %v", err)
	}
	if _, err := os.Stat(cacheLockPath(live)); err != nil {
		t.Errorf("lock of a present cache was pruned: %v", err)
	}
}

// TestLockCacheRetriesAnUnlinkedLock: a waiter whose lock file is unlinked
// while it waits must not count the dead file's lock as the single-flight
// lock; it ends up holding the file now at the path, which a peer then sees
// as held.
func TestLockCacheRetriesAnUnlinkedLock(t *testing.T) {
	cache := cachePathFor(t.TempDir(), "fp")
	first := lockCache(cache)
	got := make(chan func())
	go func() { got <- lockCache(cache) }()
	time.Sleep(100 * time.Millisecond) // let the waiter block on the old file
	if err := os.Remove(cacheLockPath(cache)); err != nil {
		t.Fatal(err)
	}
	first()
	second := <-got
	defer second()
	if _, err := os.Stat(cacheLockPath(cache)); err != nil {
		t.Fatalf("no lock file at the path after the retry: %v", err)
	}
	if !lockHeld(t, cacheLockPath(cache)) {
		t.Fatal("the waiter returned without holding the lock file at the path")
	}
}
