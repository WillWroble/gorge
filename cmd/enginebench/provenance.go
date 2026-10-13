package main

import (
	"bufio"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

// commit is the source revision this binary was built from, stamped by
// scripts/enginebench-build.sh with -ldflags "-X main.commit=<sha>" (a
// git-archive build has no .git, so the Go toolchain's own vcs stamp is
// absent there). Empty means "use debug.ReadBuildInfo's vcs.revision".
var commit string

// provenance is the build and host context a result line carries so a
// published perf history can tell a code change from a host change. Every
// field is additive to the result JSON (omitempty), so older readers and
// older lines stay compatible.
type provenance struct {
	Commit    string `json:"commit,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
	CPUModel  string `json:"cpu_model,omitempty"`
	GOGC      string `json:"gogc,omitempty"`
	Pin       string `json:"pin,omitempty"`
}

func readProvenance() provenance {
	p := provenance{
		Commit:    buildCommit(),
		GoVersion: runtime.Version(),
		GOGC:      os.Getenv("GOGC"),
		Pin:       os.Getenv("GORGE_BENCH_PIN"),
	}
	if p.GOGC == "" {
		p.GOGC = "default"
	}
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		p.CPUModel = cpuModel(f)
		f.Close()
	}
	return p
}

// buildCommit prefers the ldflag stamp, then the toolchain's vcs.revision
// (with a "-dirty" suffix when vcs.modified is set), else "".
func buildCommit() string {
	if commit != "" {
		return commit
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return vcsRevision(bi.Settings)
}

func vcsRevision(settings []debug.BuildSetting) string {
	rev, dirty := "", false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" && dirty {
		rev += "-dirty"
	}
	return rev
}

// cpuModel returns the first "model name" value of a /proc/cpuinfo stream,
// or "" when there is none (non-x86 kernels name it differently).
func cpuModel(r io.Reader) string {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok && strings.TrimSpace(k) == "model name" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
