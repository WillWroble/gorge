package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// TestAbortedCastReversesSpellWatcher drives the CR 733.1 "spell mana no
// longer payable" abort and asserts the caster's SpellsCastThisTurn watcher is
// restored to its pre-proposal value. The push's PutOnStack stays in the
// append-only log, so only abortCast's companion SpellCastReversed event can
// undo the tally the fold recorded.
func TestAbortedCastReversesSpellWatcher(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e := crAbortEngine(t, reg, "uw-control", "Dread Return", "Stoneforge Mystic", "Sanctum Prelate")
	id := crAbortMove(t, e, 0, "Lightning Bolt", state.ZHand)
	e.askPriority(0)
	e.pending = nil
	before := e.G.Players[0].SpellsCastThisTurn

	e.beginCast(0, decision.Option{Kind: "cast", Obj: id})
	e.Advance()
	if !hasNote(e, "cast aborted: cost no longer payable") {
		t.Fatal("precondition: the cast did not abort at the unpayable-mana site")
	}
	pushes, reversals := 0, 0
	for _, ev := range e.L.Events {
		switch {
		case ev.Kind == events.PutOnStack && ev.Player == 0:
			pushes++
		case ev.Kind == events.SpellCastReversed && ev.Player == 0:
			reversals++
		}
	}
	if pushes != 1 {
		t.Fatalf("precondition: %d PutOnStack events for seat 0, want 1 (the push never reached the stack, so the tally was never folded)", pushes)
	}
	if reversals != 1 {
		t.Fatalf("abortCast emitted %d SpellCastReversed events, want 1 (the reversal handler never ran)", reversals)
	}
	if got := e.G.Players[0].SpellsCastThisTurn; got != before {
		t.Errorf("aborted cast left SpellsCastThisTurn at %d, want %d (CR 733.1 must reverse the tally)", got, before)
	}
}
