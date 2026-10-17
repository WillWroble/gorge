package rules

import (
	"slices"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestAnimatedEquipmentStaysEquipment pins CR 205.1a on the real corpus
// carrier: Iron Man Armor's "{2}: ... it becomes a 0/0 Construct Hero
// artifact creature with flying" (Animate RemoveCreatureTypes$ True) sets
// its CREATURE subtypes only. The artifact subtype Equipment belongs to a
// card type the object keeps, so the animated armor is still an Equipment
// (and, as a creature, can't equip -- CR 301.5c).
func TestAnimatedEquipmentStaysEquipment(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	e := corpusEngine(t, reg, []*cards.Card{lookup(t, reg, "Iron Man Armor")}, []*cards.Card{})
	id := moveByName(t, e, 0, "Iron Man Armor", state.ZBattlefield)
	// The placement's "attach it to target creature you control" ETB has no
	// creature to target; drop it so the priority walk offers the armor.
	e.pendingTriggers = nil
	addMana(t, e, 0, "CC")
	submitChoices(t, e, animateAbilityOption(t, e, id).Index)
	settleActivation(t, e)

	d := e.Derived(id)
	for _, want := range []string{"Artifact", "Creature", "Equipment", "Construct", "Hero"} {
		if !slices.Contains(d.Types, want) {
			t.Fatalf("animated Iron Man Armor types = %v, missing %q (CR 205.1a keeps the artifact subtype)", d.Types, want)
		}
	}
}

// TestRemoveCreatureTypesKeepsOtherSubtypes is the strip's boundary on a
// direct layer effect over a creature land: the creature subtype goes, the
// land subtype stays with the Land card type (CR 205.1a).
func TestRemoveCreatureTypesKeepsOtherSubtypes(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	e := corpusEngine(t, reg, []*cards.Card{lookup(t, reg, "Dryad Arbor")}, []*cards.Card{})
	id := moveByName(t, e, 0, "Dryad Arbor", state.ZBattlefield)
	e.AddContinuous(state.ContinuousEffect{Source: id, Affects: "Card.Self", Layer: state.LType,
		RemoveCreatureTypes: true, AddTypes: []string{"Golem"}})
	d := e.Derived(id)
	if !slices.Contains(d.Types, "Forest") || slices.Contains(d.Types, "Dryad") || !slices.Contains(d.Types, "Golem") {
		t.Fatalf("types after strip+add = %v, want Forest kept, Dryad stripped, Golem added", d.Types)
	}
}
