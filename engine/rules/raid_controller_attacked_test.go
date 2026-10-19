package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// Raid's "if you attacked this turn" (Count$AttackersDeclared) is the
// CONTROLLER's attack -- Forge reads player.getCreaturesAttackedThisTurn()
// for the card's controller, and only the active player declares attackers
// (CR 508.1). Gutless Plunderer entering on its controller's opponent's turn,
// after that opponent attacked, used to read the opponent's attack as its
// own and dig.
func TestRaidCountsOnlyTheControllersAttack(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e := corpusEngine(t, reg,
		[]*cards.Card{mustCorpusCard(t, reg, "Gutless Plunderer"), mustCorpusCard(t, reg, "Grizzly Bears")},
		[]*cards.Card{mustCorpusCard(t, reg, "Grizzly Bears")})
	plunderer := moveByName(t, e, 0, "Gutless Plunderer", state.ZHand)
	bears := moveByName(t, e, 1, "Grizzly Bears", state.ZBattlefield)
	myBears := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	e.emit(events.Event{Kind: events.TurnChange, Player: 1, Amount: e.G.Turn + 1})
	if e.G.Active != 1 {
		t.Fatalf("precondition: active %d, want 1", e.G.Active)
	}
	e.emit(events.Event{Kind: events.DeclareAttackers, Player: 0, IDs: []state.ObjID{bears}})
	mine := effects.EvalCount(e, &effects.Ctx{Source: plunderer, Controller: 0}, "Count$AttackersDeclared")
	theirs := effects.EvalCount(e, &effects.Ctx{Source: bears, Controller: 1}, "Count$AttackersDeclared")
	if mine != 0 || theirs != 1 {
		t.Fatalf("AttackersDeclared on seat 1's turn after seat 1 attacked: seat 0 = %d (want 0), seat 1 = %d (want 1)", mine, theirs)
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: plunderer, From: state.ZHand, To: state.ZBattlefield})
	for _, pt := range e.pendingTriggers {
		if pt.Source == plunderer {
			t.Fatal("Gutless Plunderer's Raid trigger fired on the opponent's turn after the opponent attacked")
		}
	}
	// The contrast on the controller's own turn after it attacked.
	e.emit(events.Event{Kind: events.MoveZone, Obj: plunderer, From: state.ZBattlefield, To: state.ZHand})
	e.pendingTriggers = nil
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: e.G.Turn + 1})
	e.emit(events.Event{Kind: events.DeclareAttackers, Player: 1, IDs: []state.ObjID{myBears}})
	e.emit(events.Event{Kind: events.MoveZone, Obj: plunderer, From: state.ZHand, To: state.ZBattlefield})
	fired := false
	for _, pt := range e.pendingTriggers {
		fired = fired || pt.Source == plunderer
	}
	if !fired {
		t.Fatal("Gutless Plunderer's Raid trigger did not fire on its controller's turn after an attack")
	}
}
