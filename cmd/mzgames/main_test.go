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
			n, derived := 0, false
			hooks := bench.Hooks{
				Setup: func(x *rules.Engine) { e = x },
				Decision: func(_ int, d *decision.Decision, _ decision.Intent, _ *botpolicy.Board) error {
					v := view.ProjectDynamic(e.G, e, view.NoSeat, view.Omniscient, nil, d)
					if v.Status["next_id"] == 0 {
						t.Fatalf("decision %d: no game status", n)
					}
					for _, p := range v.Players {
						for _, c := range p.Battlefield {
							derived = derived || c.Status["base_power"] != 0
						}
					}
					n++
					return nil
				},
			}
			o, _, err := bench.PlayGame(cfg, seats, maxTurns, maxIntents, hooks)
			if err != nil || bench.IsAbort(o.StallOn) {
				t.Fatalf("%v auto=%v: %v %s %s", names, auto, err, o.StallOn, o.Livelock)
			}
			if !derived {
				t.Errorf("%v auto=%v: no derived characteristics in the dynamic view", names, auto)
			}
		}
	}
}

// TestOfferPotentialCastsClosesGap pins principle 3 for autotap: with
// OfferPotentialCasts every castable cast is offered, and playing one either
// casts it or reverses it cleanly.
func TestOfferPotentialCastsClosesGap(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	names := []string{"ubartifacts", "uw-control"}
	ds, err := loadPair(reg, names)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := measureGap(&buf, reg, names, ds, 2, 1, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), " unoffered_casts=0 ") {
		t.Fatalf("expected a gap without the offer: %s", buf.String())
	}
	buf.Reset()
	if err := measureGap(&buf, reg, names, ds, 2, 1, true); err != nil {
		t.Fatal(err)
	}
	line := strings.SplitN(buf.String(), "\n", 2)[0]
	if !strings.Contains(line, " unoffered_casts=0 ") || !strings.Contains(line, " extra_failed=0") || strings.Contains(line, " extra_offered=0 ") {
		t.Fatalf("offer did not close the gap cleanly: %s", line)
	}
}

// TestStagedPicksAreOrderInvariant stages two picks of a real multi-pick
// decision in both orders on clones: the half-made answers must be the same
// state, in the dynamic view, and a clone must carry it.
func TestStagedPicksAreOrderInvariant(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	found := 0
	for _, names := range goldenPairs {
		ds, err := loadPair(reg, names)
		if err != nil {
			t.Fatal(err)
		}
		for g := uint64(1); g <= 4; g++ {
			cfg := rules.Config{Seed: g, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
			seats, _ := newSeats(g, true)
			var e *rules.Engine
			hooks := bench.Hooks{
				Setup: func(x *rules.Engine) { e = x },
				Decision: func(_ int, d *decision.Decision, _ decision.Intent, _ *botpolicy.Board) error {
					i, j, ok := twoPicks(d)
					if !ok {
						return nil
					}
					found++
					a, b := e.CloneHypothetical(1), e.CloneHypothetical(1)
					if a.Stage(i) != nil || a.Stage(j) != nil || b.Stage(j) != nil || b.Stage(i) != nil {
						t.Fatalf("%s: staging %d and %d failed", d.Kind, i, j)
					}
					if viewKey(a) == viewKey(e) {
						t.Fatalf("%s: staged picks are not in the view", d.Kind)
					}
					if viewKey(a) != viewKey(b) {
						t.Errorf("%s: staging %d,%d and %d,%d differ in the view", d.Kind, i, j, j, i)
					}
					if c := a.Clone(); viewKey(c) != viewKey(a) || len(c.Pending().Staged) != 2 {
						t.Errorf("%s: a clone lost the staged picks", d.Kind)
					}
					return nil
				},
			}
			if _, _, err := bench.PlayGame(cfg, seats, maxTurns, maxIntents, hooks); err != nil {
				t.Fatal(err)
			}
		}
	}
	if found == 0 {
		t.Fatal("no multi-pick decision with two compatible picks in the golden games")
	}
}

// twoPicks finds two options of an unordered multi-pick decision that can be
// staged together: different objects, different groups.
func twoPicks(d *decision.Decision) (int, int, bool) {
	if d.Max < 2 || d.OrderedAnswer() {
		return 0, 0, false
	}
	for i := range d.Options {
		for j := i + 1; j < len(d.Options); j++ {
			x, y := d.Options[i], d.Options[j]
			if x.Obj != y.Obj && (x.Group == "" || x.Group != y.Group) {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}
