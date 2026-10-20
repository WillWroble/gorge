package encode_test

import (
	"testing"

	"github.com/adams-shaun/gorge/bots"
	_ "github.com/adams-shaun/gorge/bots/all"
	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/bots/mz/decks"
	"github.com/adams-shaun/gorge/bots/mz/encode"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/testkit/bench"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestCopiesAreFunctionallyIdentical plays each of two same-named lands
// from hand on a copy of the game: the states are the same on paper, so
// their functional hashes match, and their logical hashes do not, since
// the lands' IDs differ (the design doc's glossary).
func TestCopiesAreFunctionallyIdentical(t *testing.T) {
	found := 0
	play(t, func(e *rules.Engine, d *decision.Decision) {
		i, j, ok := twoCopies(d)
		if !ok {
			return
		}
		found++
		a, b := e.Clone(), e.Clone()
		if a.Submit(intent(d, i)) != nil || b.Submit(intent(d, j)) != nil {
			t.Fatal("land play refused")
		}
		ha, hb := encode.Of(a), encode.Of(b)
		if ha.Functional != hb.Functional {
			t.Errorf("turn %d: playing either %q differs functionally", e.G.Turn, d.Options[i].Label)
		}
		if ha.Logical == hb.Logical {
			t.Errorf("turn %d: playing either %q is logically identical", e.G.Turn, d.Options[i].Label)
		}
		if h := encode.Of(e); h.Functional == ha.Functional {
			t.Errorf("turn %d: a land play does not change the functional hash", e.G.Turn)
		}
	})
	if found == 0 {
		t.Fatal("no priority decision with two copies of a land to play")
	}
}

// TestHashIsAFunctionOfState hashes every decision of a game twice, and on a
// clone: same state, same hashes.
func TestHashIsAFunctionOfState(t *testing.T) {
	play(t, func(e *rules.Engine, _ *decision.Decision) {
		if h := encode.Of(e); h != encode.Of(e) || h != encode.Of(e.Clone()) {
			t.Fatalf("turn %d: hash is not a function of the state", e.G.Turn)
		}
	})
}

func play(t *testing.T, at func(*rules.Engine, *decision.Decision)) {
	reg := testutil.CorpusRegistry(t)
	names := []string{"pauper-faeries", "pauper-cawgates"}
	ds := make([][]*cards.Card, 2)
	for i, n := range names {
		d, err := decks.Load(reg, n)
		if err != nil {
			t.Fatal(err)
		}
		ds[i] = d
	}
	seats := make([]seat.Seat, 2)
	for i := range seats {
		s, err := bots.New(bots.Default, bots.Options{Seed: uint64(i + 1), AutoPayMana: true})
		if err != nil {
			t.Fatal(err)
		}
		seats[i] = s
	}
	var e *rules.Engine
	hooks := bench.Hooks{
		Setup: func(x *rules.Engine) { e = x },
		Decision: func(_ int, d *decision.Decision, _ decision.Intent, _ *botpolicy.Board) error {
			at(e, d)
			return nil
		},
	}
	cfg := rules.Config{Seed: 1, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
	if _, _, err := bench.PlayGame(cfg, seats, 50, 400000, hooks); err != nil {
		t.Fatal(err)
	}
}

func twoCopies(d *decision.Decision) (int, int, bool) {
	if d.Kind != decision.KPriority {
		return 0, 0, false
	}
	for i, o := range d.Options {
		for j := i + 1; j < len(d.Options); j++ {
			if o.Kind == "play_land" && d.Options[j].Kind == o.Kind && d.Options[j].Label == o.Label {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

func intent(d *decision.Decision, i int) decision.Intent {
	return decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{i}}
}
