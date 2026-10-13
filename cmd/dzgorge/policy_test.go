package main

import (
	"testing"

	"github.com/adams-shaun/gorge/botpolicy"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/spellbench/builtins"
	"github.com/adams-shaun/gorge/internal/testutil"
)

func TestParseSBTactical(t *testing.T) {
	for _, tc := range []struct {
		spec string
		mull botpolicy.MulliganRule
	}{
		{"sbtactical", botpolicy.MulliganCoin},
		{"sbtactical:mull", botpolicy.MulliganLands},
		{"sbtactical:keep7", botpolicy.MulliganNever},
	} {
		p, err := parsePolicy(tc.spec)
		if err != nil {
			t.Fatalf("%s: %v", tc.spec, err)
		}
		if p.Kind != "sbtactical" || p.Mull != tc.mull || p.searches() {
			t.Fatalf("%s: kind %q mull %d searches %v", tc.spec, p.Kind, p.Mull, p.searches())
		}
		st, err := p.seat(7)
		if err != nil {
			t.Fatal(err)
		}
		b, ok := st.(*builtins.Seat)
		if !ok {
			// playOne installs the planner and the refusal fallback on *builtins.Seat only.
			t.Fatalf("%s: seat is %T, want *builtins.Seat", tc.spec, st)
		}
		if !b.WantsPaymentActions() {
			t.Fatalf("%s: not an auto-pay seat", tc.spec)
		}
	}
	if _, err := parsePolicy("sbtactical:bogus"); err == nil {
		t.Fatal("sbtactical:bogus parsed")
	}
}

// TestSBTacticalPlaysAGame plays sb-tactical against bot under the London round, both
// seat orders: no error, and :keep7 never mulligans while the other side may.
func TestSBTacticalPlaysAGame(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	dc := &deckCache{reg: reg, m: map[string][]*cards.Card{
		"red":   testutil.RepoDeck(t, reg, "mono-red-goblins"),
		"green": testutil.RepoDeck(t, reg, "mono-green-stompy"),
	}}
	specs := make([]*policySpec, 2)
	for i, s := range []string{"sbtactical:keep7", "bot:mull"} {
		p, err := parsePolicy(s)
		if err != nil {
			t.Fatal(err)
		}
		p.setLookup(reg)
		specs[i] = p
	}
	if specs[0].lookup == nil {
		t.Fatal("sbtactical has no card lookup")
	}
	for leg := 0; leg < 2; leg++ {
		jb := job{game: leg, leg: leg, seed: 42, decks: [2]string{"red", "green"}, sides: [2]int{leg, 1 - leg}}
		rec, _ := playOne(jb, specs, dc, reg, 60, 5000, 2, false)
		if rec.Result == "error" {
			t.Fatalf("leg %d: error %s (stall %q)", leg, rec.Err, rec.Stall)
		}
		if rec.Turns == 0 || rec.Intents == 0 {
			t.Fatalf("leg %d: empty game %+v", leg, rec)
		}
		if m := rec.Mull[leg]; m != 0 { // seat leg holds side A (sbtactical:keep7)
			t.Fatalf("leg %d: keep7 sbtactical took %d mulligans", leg, m)
		}
		t.Logf("leg %d: %s after %d turns, %d intents, mulligans %v, %.0f ms", leg, rec.Result, rec.Turns, rec.Intents, rec.Mull, rec.MS)
	}
}
