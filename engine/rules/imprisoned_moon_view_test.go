package rules

import (
	"slices"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// Imprisoned in the Moon (real corpus script) on a real Grizzly Bears: the
// enchanted permanent is a colorless land with "{T}: Add {C}" and loses all
// other card types and abilities (CR 613.1d layer 4, 613.1e layer 5, 613.1f
// layer 6). Both the rules' derived characteristics and every seat's view
// must say it is a Land and not a Creature; the view used to project the
// printed type line ("Creature Bear").
func TestImprisonedInTheMoonViewDropsCreature(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	moon := mustCorpusCard(t, reg, "Imprisoned in the Moon")
	bearsCard := mustCorpusCard(t, reg, "Grizzly Bears")
	e := corpusEngine(t, reg, []*cards.Card{moon}, []*cards.Card{bearsCard})
	bears := moveByName(t, e, 1, "Grizzly Bears", state.ZBattlefield)
	attachCorpusAura(t, e, 0, moon, bears)

	d := e.Derived(bears)
	t.Logf("rules Derived: types=%v colors=%q P/T=%d/%d keywords=%v IsCreature=%v", d.Types, d.Colors, d.Power, d.Toughness, d.Keywords, e.IsCreature(bears))
	if e.IsCreature(bears) || slices.Contains(d.Types, "Creature") {
		t.Fatalf("rules: enchanted Bears still a creature: types %v", d.Types)
	}
	if !slices.Contains(d.Types, "Land") {
		t.Fatalf("rules: enchanted Bears not a land: types %v", d.Types)
	}

	for _, seat := range []state.PlayerID{0, 1} {
		v := view.Project(e.G, e, seat, nil)
		var cv *view.CardView
		for i := range v.Players {
			for j := range v.Players[i].Battlefield {
				if v.Players[i].Battlefield[j].ID == bears {
					cv = &v.Players[i].Battlefield[j]
				}
			}
		}
		if cv == nil {
			t.Fatalf("seat %d view: Bears not on the battlefield", seat)
		}
		t.Logf("seat %d view: types=%q P/T=%d/%d keywords=%v produces=%v", seat, cv.Types, cv.Power, cv.Toughness, cv.Keywords, cv.Produces)
		if cv.Types != "Land" {
			t.Errorf("seat %d view: Types %q, want %q", seat, cv.Types, "Land")
		}
		// The compiled facts the policy encoder reads instead of the text
		// (view.CardFacts) must describe the projected line, not the
		// printed one.
		if cv.Facts.OK {
			if fold, exact := cards.TypeLineMasks(cv.Types); cv.Facts.TypeFold != fold || cv.Facts.TypeExact != exact {
				t.Errorf("seat %d view: Facts type masks %v/%v, want TypeLineMasks(%q) = %v/%v",
					seat, cv.Facts.TypeFold, cv.Facts.TypeExact, cv.Types, fold, exact)
			}
			if cv.Facts.TypeFold&cards.TypeCreature != 0 {
				t.Errorf("seat %d view: Facts still say Creature", seat)
			}
		}
		if len(cv.Keywords) != 0 {
			t.Errorf("seat %d view: keywords %v, want none (loses all other abilities)", seat, cv.Keywords)
		}
		// P/T is not asserted: Derived keeps the printed 2/2 on a
		// noncreature (as on an uncrewed Vehicle) and the view projects it;
		// every creature-gated reader (policynet's zone totals) skips it now
		// that the type line no longer says Creature.
	}
}
