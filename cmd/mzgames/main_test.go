package main

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/bots/mz/decks"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/testkit/bench"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

var update = flag.Bool("update", false, "rewrite testdata/heads.txt")

// goldenPairs are the decks the mage fork's configs pit against each other:
// Mage.MageZero/config/config.yml and Mage.Tests/config/config.yml.
var goldenPairs = [][]string{
	{"pauper-faeries", "pauper-cawgates"},
	{"uwtempo", "standard-monob"},
}

// TestGoldenHeads is mz-gorge's regression baseline: fixed seeds, both tap
// modes, byte-identical game lines. A change that moves a line changed how
// gorge plays; run with -update only when that change is intended.
func TestGoldenHeads(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	var buf bytes.Buffer
	for _, p := range goldenPairs {
		if err := run(&buf, reg, p, 8, 1, "both", 0, false); err != nil {
			t.Fatal(err)
		}
	}
	const path = "testdata/heads.txt"
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(buf.String(), "\n")
	exp := strings.Split(string(want), "\n")
	if len(got) != len(exp) {
		t.Fatalf("got %d lines, want %d", len(got), len(exp))
	}
	for i := range got {
		if got[i] != exp[i] {
			t.Errorf("line %d\n got: %s\nwant: %s", i+1, got[i], exp[i])
		}
	}
}

// TestProjectDynamicEveryDecision projects the dynamic omniscient view at
// every decision of the golden games: it must never panic (a panic inside a
// driver hook would only show up as an aborted game).
func TestProjectDynamicEveryDecision(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	for _, names := range goldenPairs {
		ds := make([][]*cards.Card, 2)
		for i, n := range names {
			d, err := decks.Load(reg, n)
			if err != nil {
				t.Fatal(err)
			}
			ds[i] = d
		}
		for _, auto := range []bool{true, false} {
			cfg := rules.Config{Seed: 1, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
			seats, err := newSeats(1, auto)
			if err != nil {
				t.Fatal(err)
			}
			var e *rules.Engine
			n := 0
			hooks := bench.Hooks{
				Setup: func(x *rules.Engine) { e = x },
				Decision: func(_ int, d *decision.Decision, _ decision.Intent, _ *botpolicy.Board) error {
					v := view.ProjectDynamic(e.G, e, view.NoSeat, view.Omniscient, nil, d)
					if v.Status["next_id"] == 0 {
						t.Fatalf("decision %d: no game status", n)
					}
					n++
					return nil
				},
			}
			o, _, err := bench.PlayGame(cfg, seats, maxTurns, maxIntents, hooks)
			if err != nil || bench.IsAbort(o.StallOn) {
				t.Fatalf("%v auto=%v: %v %s %s", names, auto, err, o.StallOn, o.Livelock)
			}
		}
	}
}
