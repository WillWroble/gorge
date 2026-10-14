// Package memcap reads the effective cgroup v2 hard memory limit that binds
// this process. A kernel memcg OOM kill is what the reward loop's oom_kills
// stability veto counts (scripts/reward.py), so a job whose working set
// cannot fit under the limit that binds it must fail fast at startup instead
// of running until the kernel kills it and the launcher retries into another
// kill. The first consumer is cmd/cardval's az pilot (2026-10-14: the
// azfix seat retried `cardval -az -workers 4` under a 4 GiB cap seven times;
// every attempt was OOM-killed at ~4.0 GiB RSS and the burst vetoed the
// scoreboard at head 8b70eaf23).
package memcap

import (
	"bufio"
	"os"
	"path"
	"strconv"
	"strings"
)

// Limit returns the effective hard memory limit in bytes for the process
// whose cgroup is described by cgroupProc (default /proc/self/cgroup),
// reading memory.max at the cgroup and every ancestor under fsRoot (default
// /sys/fs/cgroup) and returning the minimum finite value found. It returns 0
// when no finite limit applies: no memory.max on the path, every value the
// unlimited "max", a cgroup v1 host (no "0::" line), or unreadable files.
// Every failure mode resolves to "no limit", because a failed read must
// never make a job refuse to run.
func Limit(cgroupProc, fsRoot string) int64 {
	if cgroupProc == "" {
		cgroupProc = "/proc/self/cgroup"
	}
	if fsRoot == "" {
		fsRoot = "/sys/fs/cgroup"
	}
	rel := v2Path(cgroupProc)
	best := int64(0)
	for {
		if v, ok := readMax(fsRoot + rel + "/memory.max"); ok && (best == 0 || v < best) {
			best = v
		}
		if rel == "" {
			return best
		}
		rel = parent(rel)
	}
}

// v2Path returns the unified-hierarchy cgroup path from a /proc/<pid>/cgroup
// file: the contents after the "0::" marker. "" means none was found (a v1
// host, or an unreadable file), so the caller probes only the fs root.
func v2Path(cgroupProc string) string {
	f, err := os.Open(cgroupProc)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if p, ok := strings.CutPrefix(sc.Text(), "0::"); ok {
			return p
		}
	}
	return ""
}

// parent strips one cgroup path component: "/a/b" -> "/a", "/a" -> "", and
// "/" and "" stay "".
func parent(rel string) string {
	if rel == "/" {
		return ""
	}
	d := path.Dir(rel)
	if d == "/" || d == "." {
		return ""
	}
	return d
}

// readMax parses one memory.max value. "max" (the unlimited default) and
// anything unparseable report not-ok, so the caller treats it as absent.
func readMax(p string) (int64, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	if s == "max" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}
