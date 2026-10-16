package rules

// The referent-based block-history filter words (blockedByValidThisTurn,
// blockedValidThisTurn, blockedByRemembered, blockedRemembered,
// blockedBySourceLKI). They read the event-folded this-turn block record
// (state.Object.BlockedByThisTurn / BlockedThisTurn), which names the
// creatures on the other side of each block, so they keep answering after
// the end-of-combat reset and after the named creature has died.

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

func TestBlockHistoryReferentWordsAreKnown(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{
		"Creature.blockedValidThisTurn Zombie,Creature.blockedByValidThisTurn Zombie",
		"Creature.blockedValidThisTurn Creature.Legendary,Creature.blockedByValidThisTurn Creature.Legendary",
		"Card.Self+blockedValidThisTurn Creature.Blue,Card.Self+blockedByValidThisTurn Creature.Blue",
		"Creature.blockedValidThisTurn Card.StrictlySelf",
		"Creature.blockedByValidThisTurn ParentTarget",
		"TargetedCard.Self,Creature.blockedByValidThisTurn Targeted",
		"Creature.blockedRemembered,Creature.blockedByRemembered",
		"Creature.blockedByRemembered",
		"Creature.blockedBySourceLKI",
		"Creature.!blockedByRemembered", "Creature.!blockedBySourceLKI",
	} {
		if un := effects.UnknownPredicates(spec); len(un) != 0 {
			t.Errorf("UnknownPredicates(%q) = %v, want none", spec, un)
		}
	}
	for _, spec := range []string{
		"Creature.blockedByValidThisTurn Bogus.Nope+x",
		"Creature.blockedByValidThisTurn Bogus.Nope",
		"Creature.blockedValidThisTurn Creature.NopeQual",
		"Creature.blockedByValidThisTurn",
		"Creature.blockedByRemembered Zombie",
	} {
		if un := effects.UnknownPredicates(spec); len(un) == 0 {
			t.Errorf("UnknownPredicates(%q) = none, want an unknown word", spec)
		}
	}
}

// blockHistoryPair places a blocker (controller 0) and an attacker
// (controller 1) and a bystander of the attacker's side, then declares the
// block.
func blockHistoryPair(t *testing.T, blockerName, attackerName string) (e *Engine, att, blk, bystander state.ObjID) {
	t.Helper()
	e = combatEngine(t)
	att = onBoardCard(t, e, 1, corpusCard(t, attackerName))
	blk = onBoardCard(t, e, 0, corpusCard(t, blockerName))
	bystander = onBoardCard(t, e, 1, corpusCard(t, "Grizzly Bears"))
	for _, id := range []state.ObjID{att, blk, bystander} {
		if e.G.Obj(id).Zone != state.ZBattlefield {
			t.Fatalf("precondition: %d is on the battlefield", id)
		}
	}
	e.emit(events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{att, blk}}})
	if !containsID(e.G.Obj(att).BlockedBy, blk) {
		t.Fatal("precondition: the block is on record")
	}
	return e, att, blk, bystander
}

func blockHistoryDies(e *Engine, id state.ObjID, owner state.PlayerID) {
	e.emit(events.Event{Kind: events.MoveZone, Obj: id, Player: owner, From: state.ZBattlefield, To: state.ZGraveyard})
}

func TestBlockHistoryValidArg(t *testing.T) {
	t.Parallel()
	const byZombie = "Creature.blockedByValidThisTurn Zombie"
	const zombieBlocked = "Creature.blockedValidThisTurn Zombie"

	// Blocker side named by type: a Zombie blocks a non-Zombie.
	e, att, blk, by := blockHistoryPair(t, "Walking Corpse", "Grizzly Bears")
	if !blockedByMatch(e, blk, "Creature.Zombie", 0) || blockedByMatch(e, att, "Creature.Zombie", 0) {
		t.Fatal("precondition: the blocker is a Zombie and the attacker is not")
	}
	e.emit(events.Event{Kind: events.EndCombatReset})
	if len(e.G.Obj(att).BlockedBy) != 0 {
		t.Fatal("precondition: the live BlockedBy list was reset")
	}
	if !blockedByMatch(e, att, byZombie, 0) {
		t.Fatal("blockedByValidThisTurn Zombie forgot the block after end of combat")
	}
	if blockedByMatch(e, by, byZombie, 0) {
		t.Fatal("blockedByValidThisTurn Zombie matched a creature nothing blocked")
	}
	if blockedByMatch(e, att, zombieBlocked, 0) {
		t.Fatal("blockedValidThisTurn Zombie matched the attacker, which blocked nothing")
	}
	// The Zombie dies: the attacker still reads blocked by a Zombie.
	blockHistoryDies(e, blk, 0)
	if e.G.Obj(blk).Zone != state.ZGraveyard {
		t.Fatal("precondition: the Zombie blocker died")
	}
	if !blockedByMatch(e, att, byZombie, 0) {
		t.Fatal("blockedByValidThisTurn Zombie forgot a blocker that has since died")
	}
	e.emit(events.Event{Kind: events.TurnChange, Player: 0})
	if blockedByMatch(e, att, byZombie, 0) {
		t.Fatal("TurnChange did not clear the this-turn block history")
	}

	// Mirror: a non-Zombie blocks a Zombie attacker.
	e, att, blk, by = blockHistoryPair(t, "Grizzly Bears", "Walking Corpse")
	if !blockedByMatch(e, att, "Creature.Zombie", 0) || blockedByMatch(e, blk, "Creature.Zombie", 0) {
		t.Fatal("precondition: the attacker is a Zombie and the blocker is not")
	}
	e.emit(events.Event{Kind: events.EndCombatReset})
	if !blockedByMatch(e, blk, zombieBlocked, 0) {
		t.Fatal("blockedValidThisTurn Zombie forgot the block after end of combat")
	}
	if blockedByMatch(e, by, zombieBlocked, 0) {
		t.Fatal("blockedValidThisTurn Zombie matched a creature that never blocked")
	}
	blockHistoryDies(e, att, 1)
	if e.G.Obj(att).Zone != state.ZGraveyard {
		t.Fatal("precondition: the Zombie attacker died")
	}
	// The blocker-side slice names the attacker, so its death changes nothing.
	if !blockedByMatch(e, blk, zombieBlocked, 0) {
		t.Fatal("blockedValidThisTurn Zombie forgot an attacker that has since died")
	}
	// Card.StrictlySelf resolves against the source: only the real blocked
	// creature's own source reads true.
	if !effects.MatchesObjectCtx(e.G, "Creature.blockedValidThisTurn Card.StrictlySelf", e.G.Obj(blk), effects.SpecContext{Source: att}) {
		t.Fatal("blockedValidThisTurn Card.StrictlySelf missed the blocker of the source")
	}
	if effects.MatchesObjectCtx(e.G, "Creature.blockedValidThisTurn Card.StrictlySelf", e.G.Obj(blk), effects.SpecContext{Source: by}) {
		t.Fatal("blockedValidThisTurn Card.StrictlySelf matched a blocker of another creature")
	}
	e.emit(events.Event{Kind: events.TurnChange, Player: 0})
	if blockedByMatch(e, blk, zombieBlocked, 0) {
		t.Fatal("TurnChange did not clear the blocker-side history")
	}
}

func TestBlockHistoryTargetReferent(t *testing.T) {
	t.Parallel()
	e, att, wall, by := blockHistoryPair(t, "Wall of Corpses", "Grizzly Bears")
	const spec = "Creature.blockedByValidThisTurn ParentTarget"
	bound := effects.SpecContext{ParentBound: true, ParentTargets: []state.Target{{Obj: wall}}}
	m := func(id state.ObjID, s string, sc effects.SpecContext) bool {
		return effects.MatchesObjectCtx(e.G, s, e.G.Obj(id), sc)
	}
	if !m(att, spec, bound) {
		t.Fatal("blockedByValidThisTurn ParentTarget missed the creature the target blocked")
	}
	if m(by, spec, bound) {
		t.Fatal("blockedByValidThisTurn ParentTarget matched a creature the target did not block")
	}
	// Two targets are a union (Sorrow's Path).
	two := effects.SpecContext{ParentBound: true, ParentTargets: []state.Target{{Obj: by}, {Obj: wall}}}
	if !m(att, "Creature.blockedByValidThisTurn Targeted", two) {
		t.Fatal("a second, unrelated target hid the first target's block")
	}
	// Unbound: both spellings fail closed.
	var unbound effects.SpecContext
	if m(att, spec, unbound) || m(att, "Creature.!blockedByValidThisTurn ParentTarget", unbound) {
		t.Fatal("an unbound referent matched (positive or negated)")
	}
	// The negation inverts normally once bound.
	if !m(by, "Creature.!blockedByValidThisTurn ParentTarget", bound) {
		t.Fatal("the negated spelling did not invert under a bound referent")
	}
}

func TestBlockHistoryRemembered(t *testing.T) {
	t.Parallel()
	e := combatEngine(t)
	a1 := onBoardCard(t, e, 1, corpusCard(t, "Grizzly Bears"))
	a2 := onBoardCard(t, e, 1, corpusCard(t, "Grizzly Bears"))
	by := onBoardCard(t, e, 1, corpusCard(t, "Grizzly Bears"))
	b1 := onBoardCard(t, e, 0, corpusCard(t, "Walking Corpse"))
	b2 := onBoardCard(t, e, 0, corpusCard(t, "Wall of Corpses"))
	e.emit(events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{a1, b1}, {a2, b2}}})
	e.emit(events.Event{Kind: events.EndCombatReset})
	m := func(id state.ObjID, s string, sc effects.SpecContext) bool {
		return effects.MatchesObjectCtx(e.G, s, e.G.Obj(id), sc)
	}
	rem := effects.SpecContext{Remembered: []state.Target{{Obj: b1}, {Obj: b2}}}
	const byRem = "Creature.blockedByRemembered"
	if !m(a1, byRem, rem) || !m(a2, byRem, rem) {
		t.Fatal("blockedByRemembered must match the attackers of BOTH remembered blockers")
	}
	if m(by, byRem, rem) {
		t.Fatal("blockedByRemembered matched a creature nothing blocked")
	}
	// Ride Down: the remembered blocker is destroyed first.
	blockHistoryDies(e, b1, 0)
	if e.G.Obj(b1).Zone != state.ZGraveyard {
		t.Fatal("precondition: the remembered blocker was destroyed")
	}
	if !m(a1, byRem, rem) {
		t.Fatal("blockedByRemembered forgot an attacker whose blocker was destroyed")
	}
	// The blocker side: remembered attackers.
	remAtt := effects.SpecContext{Remembered: []state.Target{{Obj: a2}}}
	if !m(b2, "Creature.blockedRemembered", remAtt) || m(b1, "Creature.blockedRemembered", remAtt) {
		t.Fatal("blockedRemembered did not pick out exactly the creature that blocked the remembered attacker")
	}
	// The source's persistent Remembered joins the union (IsRemembered's rule).
	e.G.Obj(b2).Remembered = []state.Target{{Obj: b2}}
	if !m(a2, byRem, effects.SpecContext{Source: b2}) {
		t.Fatal("the source object's Remembered is not part of the Remembered union")
	}
	// No Remembered at all: both spellings fail closed.
	var none effects.SpecContext
	if m(a1, byRem, none) || m(a1, "Creature.!blockedByRemembered", none) ||
		m(b2, "Creature.!blockedRemembered", none) {
		t.Fatal("an empty Remembered matched (positive or negated)")
	}
	// IsRemembered still agrees with the shared union.
	if !m(b1, "Creature.IsRemembered", rem) || m(by, "Creature.IsRemembered", rem) {
		t.Fatal("IsRemembered changed after the union was shared")
	}
}

func TestBlockedBySourceLKI(t *testing.T) {
	t.Parallel()
	e, att, wall, by := blockHistoryPair(t, "Wall of Corpses", "Grizzly Bears")
	e.emit(events.Event{Kind: events.DeclareAttackers, Player: 1, IDs: []state.ObjID{att, by}})
	if !e.G.Obj(att).IsAttacking || !e.G.Obj(by).IsAttacking {
		t.Fatal("precondition: both creatures are attacking")
	}
	// Wall of Corpses is sacrificed as a cost: it leaves before the target
	// is chosen, and the live BlockedBy link is tombstoned.
	blockHistoryDies(e, wall, 0)
	if e.G.Obj(wall).Zone != state.ZGraveyard {
		t.Fatal("precondition: the wall was sacrificed")
	}
	if containsID(e.G.Obj(att).BlockedBy, wall) {
		t.Fatal("precondition: the live link no longer names the wall (the case LKI exists for)")
	}
	sc := effects.SpecContext{Source: wall}
	if !effects.MatchesObjectCtx(e.G, "Creature.blockedBySourceLKI", e.G.Obj(att), sc) {
		t.Fatal("blockedBySourceLKI missed the creature the sacrificed wall was blocking")
	}
	if effects.MatchesObjectCtx(e.G, "Creature.blockedBySourceLKI", e.G.Obj(by), sc) {
		t.Fatal("blockedBySourceLKI matched a creature the wall did not block")
	}
	var unbound effects.SpecContext
	if effects.MatchesObjectCtx(e.G, "Creature.!blockedBySourceLKI", e.G.Obj(att), unbound) {
		t.Fatal("an unbound source matched the negated spelling")
	}
	e.emit(events.Event{Kind: events.EndCombatReset})
	if effects.MatchesObjectCtx(e.G, "Creature.blockedBySourceLKI", e.G.Obj(att), sc) {
		t.Fatal("blockedBySourceLKI matched after combat ended")
	}
}

// TestBlockHistoryJovensFerretsTapsItsBlockers drives the real card: at end of
// combat the creature that blocked Joven's Ferrets is tapped (TapAll over
// Creature.blockedValidThisTurn Card.StrictlySelf), and a bystander is not.
func TestBlockHistoryJovensFerretsTapsItsBlockers(t *testing.T) {
	t.Parallel()
	e := combatEngine(t)
	ferrets := onBoardCard(t, e, 1, corpusCard(t, "Joven's Ferrets"))
	blocker := onBoardCard(t, e, 0, corpusCard(t, "Grizzly Bears"))
	bystander := onBoardCard(t, e, 0, corpusCard(t, "Grizzly Bears"))
	e.emit(events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{ferrets, blocker}}})
	if e.G.Obj(blocker).Tapped || e.G.Obj(bystander).Tapped {
		t.Fatal("precondition: the creatures start untapped")
	}
	if n := stepTriggers(e, state.StepEndCombat, 1); n == 0 {
		t.Fatal("precondition: end of combat queued Joven's Ferrets' trigger")
	}
	e.putTriggersOnStack()
	if len(e.G.Stack) == 0 {
		t.Fatal("precondition: the trigger reached the stack")
	}
	e.resolveTop()
	if !e.G.Obj(blocker).Tapped {
		t.Fatal("the creature that blocked Joven's Ferrets was not tapped at end of combat")
	}
	if e.G.Obj(bystander).Tapped {
		t.Fatal("a creature that did not block Joven's Ferrets was tapped")
	}
}
