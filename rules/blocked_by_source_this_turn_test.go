package rules

// The this-turn block history (state.Object.BlockedByThisTurn /
// BlockedThisTurn) and the filter predicates that read it:
// blockedBySourceThisTurn (Defiant Vanguard's end-of-combat "destroy it and
// all creatures it blocked this turn"), the bare attacker-side spellings
// blockedByThisTurn and gotBlockedThisTurn (Heat Stroke, Fyndhorn Druid), and
// the blocker-side blockedThisTurn (Heat Stroke, Gideon's Triumph, Hezrou //
// Demonic Stench, Sizzling Barrage). Before this, effects.UnknownPredicates
// reported every one of them, so the specs failed closed and Defiant
// Vanguard's delayed trigger destroyed only itself. The tests drive the REAL
// cards through events only; the history moves through the event fold
// (DeclareBlockers), never by writing the field.

import (
	"testing"

	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

func TestBlockedByThisTurnPredicatesAreNoLongerUnknown(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{
		"Card.blockedBySourceThisTurn", "Card.!blockedBySourceThisTurn",
		"Creature.blockedByThisTurn", "Creature.!blockedByThisTurn",
		"Card.gotBlockedThisTurn", "Card.!gotBlockedThisTurn",
		"Creature.blockedThisTurn", "Creature.!blockedThisTurn",
	} {
		if un := effects.UnknownPredicates(spec); len(un) != 0 {
			t.Errorf("UnknownPredicates(%q) = %v, want none", spec, un)
		}
	}
}

func blockedByMatch(e *Engine, id state.ObjID, spec string, src state.ObjID) bool {
	return effects.MatchesObjectCtx(e.G, spec, e.G.Obj(id), effects.SpecContext{Source: src})
}

// TestBlockedByThisTurnTruthTable: a blocker and its blocked attacker are
// placed, the block is declared, and the source-relative, bare and
// gotBlockedThisTurn spellings all read true. The live BlockedBy list is
// cleared at end of combat; the history must survive it (that is the whole
// point of B10) and then clear at the turn boundary. The negated form is the
// exact inverse.
func TestBlockedByThisTurnTruthTable(t *testing.T) {
	t.Parallel()
	e := combatEngine(t)
	att := onBoardCard(t, e, 1, corpusCard(t, "Bane Alley Blackguard"))
	blk := onBoardCard(t, e, 0, corpusCard(t, "Defiant Vanguard"))
	other := onBoardCard(t, e, 0, corpusCard(t, "Grizzly Bears"))
	if e.G.Obj(att).Zone != state.ZBattlefield || e.G.Obj(blk).Zone != state.ZBattlefield {
		t.Fatal("precondition: attacker and blocker are on the battlefield")
	}

	const srcSpec = "Card.blockedBySourceThisTurn"
	const bareSpec = "Creature.blockedByThisTurn"
	const gotSpec = "Card.gotBlockedThisTurn"
	check := func(when string, want bool) {
		t.Helper()
		if got := blockedByMatch(e, att, srcSpec, blk); got != want {
			t.Fatalf("%s: blockedBySourceThisTurn(blocker) = %v, want %v", when, got, want)
		}
		// A source that did not block this attacker never matches, even when
		// the attacker was blocked by someone else.
		if blockedByMatch(e, att, srcSpec, other) {
			t.Fatalf("%s: blockedBySourceThisTurn matched an unrelated source", when)
		}
		if got := blockedByMatch(e, att, bareSpec, 0); got != want {
			t.Fatalf("%s: blockedByThisTurn = %v, want %v", when, got, want)
		}
		if got := blockedByMatch(e, att, gotSpec, 0); got != want {
			t.Fatalf("%s: gotBlockedThisTurn = %v, want %v", when, got, want)
		}
		if got := blockedByMatch(e, att, "Creature.!blockedByThisTurn", 0); got == want {
			t.Fatalf("%s: !blockedByThisTurn = %v, want %v", when, got, !want)
		}
		// The blocker side: the declared blocker reads blockedThisTurn true
		// exactly when the block is on record; a creature that never blocked
		// never matches.
		if got := blockedByMatch(e, blk, "Creature.blockedThisTurn", 0); got != want {
			t.Fatalf("%s: blockedThisTurn(blocker) = %v, want %v", when, got, want)
		}
		if blockedByMatch(e, other, "Creature.blockedThisTurn", 0) {
			t.Fatalf("%s: blockedThisTurn matched a creature that never blocked", when)
		}
	}
	check("before the block", false)

	e.emit(events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{att, blk}}})
	if !containsID(e.G.Obj(att).BlockedBy, blk) {
		t.Fatal("precondition: the block was recorded live")
	}
	check("after the block", true)

	// End of combat empties the live list; the history must remain, or the
	// end-of-combat delayed trigger cannot name the creature it blocked.
	e.emit(events.Event{Kind: events.EndCombatReset})
	if len(e.G.Obj(att).BlockedBy) != 0 {
		t.Fatal("precondition: EndCombatReset cleared the live BlockedBy list")
	}
	if !blockedByMatch(e, att, srcSpec, blk) {
		t.Fatal("blockedBySourceThisTurn forgot the block at end of combat; the live list cannot be its source")
	}
	check("after the end-of-combat reset", true)

	e.emit(events.Event{Kind: events.TurnChange, Player: 0})
	check("after the turn boundary", false)
}

// TestBlockedByThisTurnDoesNotSurviveBlink: CR 400.7. A creature that was
// blocked and then left and re-entered is a new object that was not blocked
// this turn.
func TestBlockedByThisTurnDoesNotSurviveBlink(t *testing.T) {
	t.Parallel()
	e := combatEngine(t)
	att := onBoardCard(t, e, 1, corpusCard(t, "Bane Alley Blackguard"))
	blk := onBoardCard(t, e, 0, corpusCard(t, "Defiant Vanguard"))
	e.emit(events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{att, blk}}})
	if !blockedByMatch(e, att, "Card.blockedBySourceThisTurn", blk) {
		t.Fatal("precondition: the attacker was blocked this turn")
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: att, Player: 1, From: state.ZBattlefield, To: state.ZGraveyard})
	e.emit(events.Event{Kind: events.MoveZone, Obj: att, Player: 1, From: state.ZGraveyard, To: state.ZBattlefield})
	if e.G.Obj(att).Zone != state.ZBattlefield {
		t.Fatal("precondition: the attacker re-entered the battlefield")
	}
	if blockedByMatch(e, att, "Card.blockedBySourceThisTurn", blk) ||
		blockedByMatch(e, att, "Card.blockedByThisTurn", 0) {
		t.Fatal("a creature that left and re-entered still reads as blocked this turn")
	}
	// The blocker half clears on the same battlefield exit.
	if !blockedByMatch(e, blk, "Creature.blockedThisTurn", 0) {
		t.Fatal("precondition: the blocker read blockedThisTurn before its own blink")
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: blk, Player: 0, From: state.ZBattlefield, To: state.ZGraveyard})
	e.emit(events.Event{Kind: events.MoveZone, Obj: blk, Player: 0, From: state.ZGraveyard, To: state.ZBattlefield})
	if e.G.Obj(blk).Zone != state.ZBattlefield {
		t.Fatal("precondition: the blocker re-entered the battlefield")
	}
	if blockedByMatch(e, blk, "Creature.blockedThisTurn", 0) {
		t.Fatal("a blocker that left and re-entered still reads as having blocked this turn")
	}
}

// TestBlockedBySourceThisTurnGatesDefiantVanguardEndOfCombat: the real card
// text, "When Defiant Vanguard blocks, at end of combat, destroy it and all
// creatures it blocked this turn." The delayed trigger's DestroyAll names
// Card.Self plus Card.blockedBySourceThisTurn, so both the Vanguard and the
// creature it blocked must reach the graveyard.
func TestBlockedBySourceThisTurnGatesDefiantVanguardEndOfCombat(t *testing.T) {
	t.Parallel()
	e := combatEngine(t)
	vanguard := onBoardCard(t, e, 0, corpusCard(t, "Defiant Vanguard"))
	attacker := onBoardCard(t, e, 1, corpusCard(t, "Bane Alley Blackguard"))
	// Declare the block directly (the trigger path, not the block legality
	// path, is what this test owns); the Vanguard's Blocks trigger registers
	// the end-of-combat delayed trigger when it resolves.
	e.emit(events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{attacker, vanguard}}})
	if !effects.MatchesObjectCtx(e.G, "Card.blockedBySourceThisTurn", e.G.Obj(attacker),
		effects.SpecContext{Source: vanguard}) {
		t.Fatal("precondition: the attacker was blocked by the Vanguard this turn")
	}
	e.putTriggersOnStack()
	if len(e.G.Stack) == 0 {
		t.Fatal("precondition: the Blocks trigger reached the stack")
	}
	e.resolveTop()
	if len(e.G.Delayed) == 0 {
		t.Fatal("precondition: resolving the Blocks trigger registered the end-of-combat delayed trigger")
	}
	// Fire the delayed trigger registered by the block.
	if n := stepTriggers(e, state.StepEndCombat, 1); n == 0 {
		t.Fatal("precondition: entering end of combat queued the delayed trigger")
	}
	e.putTriggersOnStack()
	if len(e.G.Stack) == 0 {
		t.Fatal("precondition: the end-of-combat delayed trigger reached the stack")
	}
	e.resolveTop()
	if e.G.Obj(vanguard).Zone != state.ZGraveyard {
		t.Fatalf("Defiant Vanguard zone = %v, want graveyard", e.G.Obj(vanguard).Zone)
	}
	if e.G.Obj(attacker).Zone != state.ZGraveyard {
		t.Fatalf("the creature Defiant Vanguard blocked zone = %v, want graveyard (blockedBySourceThisTurn matched nothing)", e.G.Obj(attacker).Zone)
	}
}
