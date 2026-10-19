package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// Morbid's "a creature died this turn" (Slumbering Cerberus:
// Count$ThisTurnEntered_Graveyard_from_Battlefield_Creature) reads the
// permanent as it last existed on the battlefield (CR 603.10a, CR 700.4):
// an animated Soulstone Sanctuary that dies was a creature, and a Grizzly
// Bears made a non-creature land by Imprisoned in the Moon was not. The
// count used to test the card in the graveyard, getting both backwards.
func TestMorbidReadsTheDepartedPermanentsCreatureStatus(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Slumbering Cerberus", "Soulstone Sanctuary", "Grizzly Bears", "Imprisoned in the Moon")
	cerberus := moveByName(t, e, 0, "Slumbering Cerberus", state.ZBattlefield)
	morbid := func() int32 {
		return effects.EvalCount(e, &effects.Ctx{Source: cerberus, Controller: 0}, "Count$ThisTurnEntered_Graveyard_from_Battlefield_Creature")
	}
	if n := morbid(); n != 0 {
		t.Fatalf("precondition: morbid count %d, want 0", n)
	}

	bears := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	lossAttach(t, e, "Imprisoned in the Moon", 0, bears)
	if e.IsCreature(bears) {
		t.Fatalf("precondition: imprisoned Bears still a creature: %v", e.Derived(bears).Types)
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: bears, From: state.ZBattlefield, To: state.ZGraveyard})
	if n := morbid(); n != 0 {
		t.Fatalf("a non-creature land (the imprisoned Bears) left the battlefield: morbid count %d, want 0", n)
	}

	soul := moveByName(t, e, 0, "Soulstone Sanctuary", state.ZBattlefield)
	addMana(t, e, 0, "CCCC")
	d := e.Pending()
	idx := -1
	for _, o := range d.Options {
		if o.Kind == "ability" && o.Obj == soul {
			idx = o.Index
		}
	}
	if idx < 0 {
		t.Fatalf("no animate option: %+v", d.Options)
	}
	submitChoices(t, e, idx)
	fdnCensusDrain(t, e)
	if !e.IsCreature(soul) {
		t.Fatalf("precondition: Soulstone not a creature after animating: %v", e.Derived(soul).Types)
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: soul, From: state.ZBattlefield, To: state.ZGraveyard})
	if n := morbid(); n != 1 {
		t.Fatalf("an animated Soulstone Sanctuary died: morbid count %d, want 1", n)
	}
}
