package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

func autopayAbilityOffer(d *decision.Decision, obj state.ObjID) (decision.Option, bool) {
	for _, o := range d.Options {
		if o.Kind == "ability" && o.Obj == obj {
			return o, true
		}
	}
	return decision.Option{}, false
}

// TestAbilityOfferedAndAutoPaidFromUntappedLands: Spectral Sailor's
// "{3}{U}: Draw a card" over four untapped Islands and an empty pool is an
// offered option (the census' pool-priced gap), and choosing it taps the
// lands through the planner witness and puts the ability on the stack.
func TestAbilityOfferedAndAutoPaidFromUntappedLands(t *testing.T) {
	t.Parallel()
	e, _, _ := newFixtureDeck(t, 9611, ppFiller)
	e.paidManaCensus = true
	sailor := onBoardCard(t, e, 0, corpusCard(t, "Spectral Sailor"))
	var lands []state.ObjID
	for i := 0; i < 4; i++ {
		lands = append(lands, onBoard(t, e, 0, lrIsland))
	}
	d := ppPriority(t, e)
	if e.G.Players[0].Pool.Total() != 0 {
		t.Fatalf("precondition: pool %v not empty", e.G.Players[0].Pool)
	}
	for _, l := range lands {
		if o := e.G.Obj(l); o.Tapped || o.Zone != state.ZBattlefield {
			t.Fatalf("precondition: land %d tapped=%v zone=%v", l, o.Tapped, o.Zone)
		}
	}
	opt, ok := autopayAbilityOffer(d, sailor)
	if !ok {
		t.Fatalf("Spectral Sailor's draw not offered with untapped lands and an empty pool: %+v", d.Options)
	}
	stackBefore := len(e.G.Stack)
	if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{opt.Index}}); err != nil {
		t.Fatal(err)
	}
	if got := len(e.G.Stack); got != stackBefore+1 {
		t.Fatalf("stack %d -> %d, want the ability on the stack (pending %+v)", stackBefore, got, e.Pending())
	}
	tapped := 0
	for _, l := range lands {
		if e.G.Obj(l).Tapped {
			tapped++
		}
	}
	if tapped != 4 {
		t.Fatalf("tapped %d lands, want 4", tapped)
	}
}

// TestAbilityNotOfferedWhenSourcesCannotPay: three Islands cannot pay
// {3}{U}, so the autopay offer must not appear (the gate is the planner's
// witness, not "any untapped land").
func TestAbilityNotOfferedWhenSourcesCannotPay(t *testing.T) {
	t.Parallel()
	e, _, _ := newFixtureDeck(t, 9612, ppFiller)
	e.paidManaCensus = true
	sailor := onBoardCard(t, e, 0, corpusCard(t, "Spectral Sailor"))
	for i := 0; i < 3; i++ {
		onBoard(t, e, 0, lrIsland)
	}
	d := ppPriority(t, e)
	if _, ok := autopayAbilityOffer(d, sailor); ok {
		t.Fatalf("offered with three Islands: %+v", d.Options)
	}
}
