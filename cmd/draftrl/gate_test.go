package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

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
