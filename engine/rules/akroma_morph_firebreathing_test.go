package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// Akroma, Angel of Fury's {R} firebreathing exists only on its printed face
// (CR 708.2: a face-down permanent has no abilities). The corpus census saw it
// potential in 6 games and never offered: the bot casts it face-down via morph,
// so the ability is neither offered nor potential until the turn-up. This pins
// both halves: none while face down, offered with {R} once turned face up.
func TestAkromaFirebreathingOnlyAfterTurnFaceUp(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	e, _ := manifestEngine(t, reg, "Akroma, Angel of Fury")
	id := morphDownCast(t, e, "Akroma, Angel of Fury", "morphed", "CCCCCCRRRR", 7)
	if o := e.G.Obj(id); !o.FaceDown || o.Zone != state.ZBattlefield {
		t.Fatalf("precondition: Akroma faceDown=%v zone=%v, want face-down on the battlefield", o.FaceDown, o.Zone)
	}
	if hasActivateOption(e, id) || covPotentialAbility(e, id, 0) {
		t.Fatalf("face-down Akroma offered or projected its printed {R} ability")
	}
	for _, o := range e.Pending().Options {
		if o.Kind == "ability" && o.Obj == id {
			t.Fatalf("face-down Akroma offered an ability: %+v", o)
		}
	}
	submitChoices(t, e, turnFaceUpIndex(t, e, id))
	if e.G.Obj(id).FaceDown {
		t.Fatalf("precondition: still face down after the turn-up")
	}
	if got := e.G.Players[0].Pool[state.MR]; got < 1 {
		t.Fatalf("precondition: pool R = %d, want >= 1 left for the firebreathing", got)
	}
	found := false
	for _, o := range e.Pending().Options {
		if o.Kind == "ability" && o.Obj == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("face-up Akroma's {R} ability not offered: %+v", e.Pending().Options)
	}
}
