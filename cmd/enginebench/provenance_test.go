package main

import (
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
)

func TestCPUModelParsesFirstModelName(t *testing.T) {
	in := "processor\t: 0\nvendor_id\t: AuthenticAMD\nmodel name\t: AMD Ryzen 9 7950X 16-Core Processor\n\nprocessor\t: 1\nmodel name\t: other\n"
	if got, want := cpuModel(strings.NewReader(in)), "AMD Ryzen 9 7950X 16-Core Processor"; got != want {
		t.Fatalf("cpuModel = %q, want %q", got, want)
	}
	if got := cpuModel(strings.NewReader("processor : 0\nHardware : foo\n")); got != "" {
		t.Fatalf("cpuModel without model name = %q, want empty", got)
	}
}

func TestBuildCommitPrefersLdflag(t *testing.T) {
	old := commit
	defer func() { commit = old }()
	commit = "abc123"
	if got := buildCommit(); got != "abc123" {
		t.Fatalf("buildCommit = %q, want the ldflag value", got)
	}
	got := vcsRevision([]debug.BuildSetting{{Key: "vcs.revision", Value: "deadbeef"}, {Key: "vcs.modified", Value: "true"}})
	if got != "deadbeef-dirty" {
		t.Fatalf("vcsRevision = %q, want deadbeef-dirty", got)
	}
}

// TestResultProvenanceIsAdditive: the provenance fields are new top-level
// keys next to the existing ones, so an older reader keeps working.
func TestResultProvenanceIsAdditive(t *testing.T) {
	t.Setenv("GOGC", "")
	t.Setenv("GORGE_BENCH_PIN", "7")
	r := result{Label: "x", Row: "clone", provenance: readProvenance()}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"label", "row", "heap_live_mb", "cpu_secs", "go_version", "gogc", "pin"} {
		if _, ok := m[k]; !ok {
			t.Errorf("result JSON lacks %q: %s", k, b)
		}
	}
	if m["gogc"] != "default" || m["pin"] != "7" {
		t.Errorf("gogc/pin = %v/%v, want default/7", m["gogc"], m["pin"])
	}
}
