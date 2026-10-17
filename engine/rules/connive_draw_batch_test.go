package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// Connive N is ONE "draw N cards" instruction (CR 702.59a), so its draws are
// offered to the R:Event$ DrawCards class as one batch before any card
// moves: the opponent's Alms Collector ("if an opponent would draw two or
// more cards, instead you and that player each draw a card") replaces a
// connive-2's draws as one event. A connive-1 is a one-card batch, which
// Alms Collector's Number$ GE2 gate rejects.

// conniveVaultDrawDeltas activates Spymaster's Vault after died creatures
// have died this turn (X), targeting a fresh Grizzly Bears, with Alms
// Collector on seat 1's battlefield when alms is set. It returns both seats'
// hand deltas measured at the connive's discard ask (after the draws, before
// the discard).
func conniveVaultDrawDeltas(t *testing.T, died int, alms bool) (int, int) {
	t.Helper()
	reg := testutil.CorpusRegistry(t)
	if searchTestRegistry(t) != reg {
		t.Fatal("precondition: the connive fixtures must share testutil.CorpusRegistry")
	}
	var opp []string
	if alms {
		opp = []string{"Alms Collector"}
	}
	e, _ := conniveEngine(t, []string{"Spymaster's Vault", "Grizzly Bears", "Grizzly Bears", "Grizzly Bears", "Grizzly Bears"}, opp)
	if alms {
		ac := conniveMoveTo(t, e, 1, "Alms Collector", state.ZBattlefield)
		if o := e.G.Obj(ac); o == nil || o.Zone != state.ZBattlefield || o.Controller != 1 {
			t.Fatalf("precondition: Alms Collector must be on seat 1's battlefield: %+v", o)
		}
	}
	vault := conniveMoveTo(t, e, 0, "Spymaster's Vault", state.ZBattlefield)
	e.emit(events.Event{Kind: events.Untap, Obj: vault})
	e.pending = nil
	e.priorityRound()
	for i := 0; i < died; i++ {
		bear := conniveMoveTo(t, e, 0, "Grizzly Bears", state.ZBattlefield)
		e.emit(events.Event{Kind: events.MoveZone, Obj: bear, From: state.ZBattlefield, To: state.ZGraveyard})
		e.pending = nil
		e.priorityRound()
	}
	conniver := conniveMoveTo(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	addMana(t, e, 0, "B")
	opt := abilityOption(t, e, vault, 1)
	submitChoices(t, e, opt.Index)
	d := e.Pending()
	if d == nil || d.Kind != decision.KTarget {
		t.Fatalf("after activation pending = %+v, want the target ask", d)
	}
	tIdx := -1
	for _, o := range d.Options {
		if o.Obj == conniver {
			tIdx = o.Index
		}
	}
	if tIdx < 0 {
		t.Fatalf("target ask offers no conniver %d: %+v", conniver, d.Options)
	}
	submitChoices(t, e, tIdx)
	hand0, hand1 := len(e.G.Zone(state.ZHand, 0)), len(e.G.Zone(state.ZHand, 1))
	if hand0 <= died {
		t.Fatalf("precondition: seat 0's hand (%d) must exceed the discard count so the discard is a real ask", hand0)
	}
	d = passUntilNonPriority(t, e, 30)
	if d == nil || d.Kind != decision.KModes || d.ResumeKind != "connive" || d.Min != died {
		t.Fatalf("pending = %+v, want the connive-%d discard ask", d, died)
	}
	return len(e.G.Zone(state.ZHand, 0)) - hand0, len(e.G.Zone(state.ZHand, 1)) - hand1
}

// TestConniveTwoBatchMeetsAlmsCollector: connive 2 under the opponent's Alms
// Collector draws ONE card for the conniver's controller and one for Alms
// Collector's controller -- the two-card batch replaced as one event.
func TestConniveTwoBatchMeetsAlmsCollector(t *testing.T) {
	t.Parallel()
	d0, d1 := conniveVaultDrawDeltas(t, 2, true)
	if d0 != 1 {
		t.Fatalf("seat 0 hand delta = %d, want 1 (Alms Collector replaced the connive-2 batch)", d0)
	}
	if d1 != 1 {
		t.Fatalf("seat 1 hand delta = %d, want 1 (Alms Collector's controller draws too)", d1)
	}
}

// TestConniveOneIgnoresAlmsCollector: a connive-1 is a one-card batch, which
// Alms Collector's Number$ GE2 gate rejects -- the conniver's controller
// draws its card and Alms Collector's controller draws nothing.
func TestConniveOneIgnoresAlmsCollector(t *testing.T) {
	t.Parallel()
	d0, d1 := conniveVaultDrawDeltas(t, 1, true)
	if d0 != 1 || d1 != 0 {
		t.Fatalf("hand deltas = (%d, %d), want (1, 0): a one-card connive is not a two-or-more draw", d0, d1)
	}
}

// TestConniveTwoWithoutAlmsCollectorDrawsBoth is the no-replacement
// control: the batch survives and both cards are drawn.
func TestConniveTwoWithoutAlmsCollectorDrawsBoth(t *testing.T) {
	t.Parallel()
	d0, d1 := conniveVaultDrawDeltas(t, 2, false)
	if d0 != 2 || d1 != 0 {
		t.Fatalf("hand deltas = (%d, %d), want (2, 0)", d0, d1)
	}
}
