package memcap

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const gib = int64(1) << 30

func TestLimitTakesTheMinimumAlongTheAncestorChain(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "t0/leaf/memory.max"), "4294967296\n") // 4 GiB leaf
	write(t, filepath.Join(root, "t0/memory.max"), "max\n")             // unlimited above
	write(t, filepath.Join(root, "memory.max"), "max\n")
	proc := filepath.Join(t.TempDir(), "cgroup")
	write(t, proc, "12:cpu:/x\n0::/t0/leaf\n")
	if got := Limit(proc, root); got != 4*int64(1<<30) {
		t.Fatalf("Limit = %d, want the leaf's 4 GiB", got)
	}
}

func TestLimitFindsAnAncestorLimitTheLeafDoesNotHave(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "t0/leaf/cgroup.procs"), "") // leaf exists, no memory.max
	write(t, filepath.Join(root, "t0/memory.max"), "1073741824\n")
	proc := filepath.Join(t.TempDir(), "cgroup")
	write(t, proc, "0::/t0/leaf\n")
	if got := Limit(proc, root); got != gib {
		t.Fatalf("Limit = %d, want the ancestor's 1 GiB", got)
	}
}

func TestLimitIsZeroWhenNoFiniteLimitApplies(t *testing.T) {
	// Every memory.max on the path is "max": unconfined.
	root := t.TempDir()
	write(t, filepath.Join(root, "t0/leaf/memory.max"), "max\n")
	write(t, filepath.Join(root, "memory.max"), "max\n")
	proc := filepath.Join(t.TempDir(), "cgroup")
	write(t, proc, "0::/t0/leaf\n")
	if got := Limit(proc, root); got != 0 {
		t.Fatalf("Limit = %d with only \"max\" values, want 0", got)
	}

	// A v1-style cgroup file has no "0::" line: no limit is assumed.
	proc = filepath.Join(t.TempDir(), "cgroup")
	write(t, proc, "12:memory:/x\n")
	if got := Limit(proc, root); got != 0 {
		t.Fatalf("Limit = %d for a v1 cgroup file, want 0", got)
	}

	// Garbage values are skipped, not fatal.
	root = t.TempDir()
	write(t, filepath.Join(root, "t0/memory.max"), "not-a-number\n")
	proc = filepath.Join(t.TempDir(), "cgroup")
	write(t, proc, "0::/t0\n")
	if got := Limit(proc, root); got != 0 {
		t.Fatalf("Limit = %d for a garbage memory.max, want 0", got)
	}

	// A missing cgroup file resolves to "no limit", never an error.
	if got := Limit(filepath.Join(t.TempDir(), "absent"), root); got != 0 {
		t.Fatalf("Limit = %d for a missing cgroup file, want 0", got)
	}
}

func TestLimitResolvesTheRootCgroupSelf(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "memory.max"), "536870912\n") // 512 MiB at the root
	proc := filepath.Join(t.TempDir(), "cgroup")
	write(t, proc, "0::/\n")
	if got := Limit(proc, root); got != 512*1024*1024 {
		t.Fatalf("Limit = %d, want the root's 512 MiB", got)
	}
}

func TestThisProcessReadsALimitOrNone(t *testing.T) {
	// The real /proc/self/cgroup + /sys/fs/cgroup pair must parse without
	// panicking and return a sane value: either a positive cap or 0.
	got := Limit("", "")
	if got < 0 {
		t.Fatalf("Limit = %d, want positive or 0", got)
	}
	t.Logf("this process's effective cgroup cap: %d bytes (%d MiB)", got, got>>20)
}
