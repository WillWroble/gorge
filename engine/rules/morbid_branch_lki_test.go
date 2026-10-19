package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// Count$Morbid.<yes>.<no> (Cackling Prowler, Needletooth Pack) reads a
// departed permanent's creature status as it last existed on the
// battlefield (CR 603.10a, 700.4), the bit TestMorbidReadsTheDepartedPermanents-
// CreatureStatus pins for the ThisTurnEntered spelling: an animated
// Soulstone Sanctuary that died counts, an imprisoned Bears (a land) does
// not. Census pair 2691.
func TestMorbidBranchReadsTheDepartedPermanentsCreatureStatus(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Cackling Prowler", "Soulstone Sanctuary", "Grizzly Bears", "Imprisoned in the Moon")
	prowler := moveByName(t, e, 0, "Cackling Prowler", state.ZBattlefield)
	morbid := func() int32 {
		return effects.EvalCount(e, &effects.Ctx{Source: prowler, Controller: 0}, "Count$Morbid.1.0")
	}
	if n := morbid(); n != 0 {
		t.Fatalf("precondition: morbid %d, want 0", n)
	}

	bears := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	lossAttach(t, e, "Imprisoned in the Moon", 0, bears)
	if e.IsCreature(bears) {
		t.Fatalf("precondition: imprisoned Bears still a creature: %v", e.Derived(bears).Types)
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: bears, From: state.ZBattlefield, To: state.ZGraveyard})
	if n := morbid(); n != 0 {
		t.Fatalf("a non-creature land (the imprisoned Bears) died: morbid %d, want 0", n)
	}

	soul := moveByName(t, e, 0, "Soulstone Sanctuary", state.ZBattlefield)
	addMana(t, e, 0, "CCCC")
	idx := -1
	for _, o := range e.Pending().Options {
		if o.Kind == "ability" && o.Obj == soul {
			idx = o.Index
		}
	}
	if idx < 0 {
		t.Fatal("no animate option")
	}
	submitChoices(t, e, idx)
	fdnCensusDrain(t, e)
	if !e.IsCreature(soul) {
		t.Fatalf("precondition: Soulstone not a creature after animating: %v", e.Derived(soul).Types)
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: soul, From: state.ZBattlefield, To: state.ZGraveyard})
	if n := morbid(); n != 1 {
		t.Fatalf("an animated Soulstone Sanctuary died: morbid %d, want 1", n)
	}
}
