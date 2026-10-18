package rules

import (
	"github.com/adams-shaun/gorge/engine/state"
	"testing"
)

// CR 113.6: a static ability functions only in the zone its EffectZone$
// names. Muldrotha's "cast a permanent spell of each type from your
// graveyard" names EffectZone$ Battlefield, so Muldrotha in the graveyard
// (with no Muldrotha on the battlefield) grants nothing -- in particular not
// a cast of itself from there.
func TestMuldrothaInGraveyardGrantsNothing(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Muldrotha, the Gravetide")
	mul := moveByName(t, e, 0, "Muldrotha, the Gravetide", state.ZGraveyard)
	addMana(t, e, 0, "BGUCCC")
	for _, o := range castOptions(t, e) {
		if o.Obj == mul {
			t.Fatalf("Muldrotha in the graveyard (no Muldrotha on the battlefield) is castable: %+v", o)
		}
	}
}
