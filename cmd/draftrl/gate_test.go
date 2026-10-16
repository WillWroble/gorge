package main

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// withSoftLimit pins the runtime soft memory limit for one test, so the az
// memory guard sees a known GOMEMLIMIT whatever the test was launched with.
func withSoftLimit(t *testing.T, b int64) {
	prev := debug.SetMemoryLimit(b)
	t.Cleanup(func() { debug.SetMemoryLimit(prev) })
}

// TestGateSmokeBot is DR-4's bot smoke: 1 event, 1 human deck, 1 game per
// pairing, the scripted bot pilot. Every seat's built and random deck
// plays exactly one game and the contract lines print.
func TestGateSmokeBot(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	var out bytes.Buffer
	r, err := gate([]string{"-corpus", dir, "-events", "1", "-games", "1", "-max-human", "1", "-pilot", "bot"}, &out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if r.BuiltPooled.Games != 4 || r.RandomPooled.Games != 4 || r.Games != 8 {
		t.Fatalf("games built %d random %d total %d, want 4/4/8", r.BuiltPooled.Games, r.RandomPooled.Games, r.Games)
	}
	for _, want := range []string{"\n  built:  ", "\n  random:  ", "ci95 ["} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, out.String())
		}
	}
}

// TestGateSmokeAZNoNet is DR-4's az smoke: the no-network closed-list az
// pilot at sims 2 over a small default belief, one game per seat, mixed
// per-seat builders (random on seat 1) and no baseline.
func TestGateSmokeAZNoNet(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	// 1 GiB soft holds 4 workers at sims 2 and fits the capped test scope's
	// hard limit at the guard's 1.4x headroom (a 3 GiB GOMEMLIMIT under a
	// 4G MemoryMax does not).
	withSoftLimit(t, 1<<30)
	var out bytes.Buffer
	r, err := gate([]string{"-corpus", dir, "-events", "1", "-games", "1", "-max-human", "1",
		"-pilot", "az", "-az-sims", "2", "-az-belief", "4", "-baseline", "none",
		"-builders", "17l,random,17l,17l"}, &out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if r.BuiltPooled.Games != 4 || r.RandomPooled.Games != 0 {
		t.Fatalf("games built %d random %d, want 4/0", r.BuiltPooled.Games, r.RandomPooled.Games)
	}
	if r.AZ == nil || r.AZ.Decisions == 0 {
		t.Fatalf("az pilot saw no decisions: %+v\n%s", r.AZ, out.String())
	}
}

func TestParseSeatAgents(t *testing.T) {
	a, err := parseSeatAgents("17l,ovr:/x/v.json#gih,17l,random", true)
	if err != nil || len(a) != 4 || a[1].path != "/x/v.json" || a[1].field != "gih" || a[3].kind != "random" {
		t.Fatalf("%v %+v", err, a)
	}
	for _, bad := range []string{"17l,17l", "random", "ovr:x", "nope"} {
		if _, err := parseSeatAgents(bad, false); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

// TestGateRefusesAZUnderMemory is the az memory guard's wiring: an az gate
// whose GOMEMLIMIT cannot hold its workers is refused before the corpus
// loads (the corpus path does not exist, so reaching draftlab.Open would
// fail differently), and the bot pilot never consults the guard.
func TestGateRefusesAZUnderMemory(t *testing.T) {
	withSoftLimit(t, 512<<20)
	missing := t.TempDir() + "/no-corpus"
	_, err := gate([]string{"-corpus", missing, "-pilot", "az", "-workers", "4", "-az-sims", "25"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "GOMEMLIMIT") || !strings.Contains(err.Error(), "-workers") {
		t.Fatalf("az under 512 MiB: err %v, want the memcap refusal", err)
	}
	_, err = gate([]string{"-corpus", missing, "-pilot", "bot", "-workers", "4"}, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "GOMEMLIMIT") {
		t.Fatalf("bot under 512 MiB: err %v, want the corpus error, not the az refusal", err)
	}
}
