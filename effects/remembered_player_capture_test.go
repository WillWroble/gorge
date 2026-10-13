package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/state"
)

// lb-remowner: every Remembered* PLAYER derivation reads Forge's host
// remembered list (hostRemembered), never the firing trigger's capture. A
// leave trigger's Ctx is seeded Remembered == Captured == the source itself;
// when the source wrote nothing (Unidentified Hovership whose ETB exiled
// nothing) those spellings name nobody (CR 608.2b), while the plain
// Remembered read keeps its documented capture fallback.

// capturedSelfHost builds a 3-seat host whose resolving Ctx is a leave
// trigger of a source owned and controlled by seat 1: the capture is the
// source alone and the source's persistent Remembered list is empty.
func capturedSelfHost(t *testing.T) (*fakeHost, *Ctx, *state.Object) {
	t.Helper()
	h := newHost(t, 3)
	src := h.g.AddObject(mkCard(t, "Name:Leaving Vehicle\nTypes:Artifact Vehicle\nPT:2/2\nOracle:x\n"), 1)
	src.Controller = 1
	self := []state.Target{{Obj: src.ID}}
	c := &Ctx{Source: src.ID, Controller: 1, Remembered: copyTargets(self), Captured: copyTargets(self)}
	if len(src.Remembered) != 0 {
		t.Fatalf("precondition: source wrote %v, want an empty persistent list", src.Remembered)
	}
	if got := PlayerOf(h, c, self[0]); got != 1 {
		t.Fatalf("precondition: captured source maps to seat %d, want 1", got)
	}
	return h, c, src
}

func TestRememberedPlayerSpellingsIgnoreTriggerCapture(t *testing.T) {
	h, c, src := capturedSelfHost(t)

	// The plain Remembered read keeps the ordinary-trigger capture fallback
	// (resolvedRemembered's second shape) -- the precondition that the
	// capture is really there for the player spellings to drop.
	if got := DefinedSpec(h, c, "Remembered"); len(got) != 1 || got[0].Obj != src.ID {
		t.Fatalf("precondition: Defined$ Remembered = %+v, want the capture [%d]", got, src.ID)
	}
	for _, sel := range []string{"RememberedOwner", "RememberedController", "NonRememberedController", "OppNonRememberedController"} {
		wantPlayers(t, definedPlayerIDs(h, c, sel), nil, "definedPlayerIDs("+sel+") with nothing remembered")
	}

	// Once the source WROTE its list (the ETB exiled seat 2's card), the
	// owner/controller spellings name that card's seat, not the capture's.
	exiled := h.g.AddObject(mkCard(t, "Name:Exiled Bear\nTypes:Creature Bear\nPT:2/2\nOracle:x\n"), 2)
	exiled.Controller = 2
	h.g.Obj(src.ID).Remembered = []state.Target{{Obj: exiled.ID}}
	if got := h.g.Obj(c.Source).Remembered; len(got) != 1 || got[0].Obj != exiled.ID {
		t.Fatalf("precondition: source persistent list = %+v, want [%d]", got, exiled.ID)
	}
	wantPlayers(t, definedPlayerIDs(h, c, "RememberedOwner"), []state.PlayerID{2}, "definedPlayerIDs(RememberedOwner) after the write")
	wantPlayers(t, definedPlayerIDs(h, c, "RememberedController"), []state.PlayerID{2}, "definedPlayerIDs(RememberedController) after the write")
	wantPlayers(t, definedPlayerIDs(h, c, "NonRememberedController"), []state.PlayerID{1, 0}, "definedPlayerIDs(NonRememberedController) after the write")
}

func TestRememberedPlayerIgnoresTriggerPlayerCapture(t *testing.T) {
	h := newHost(t, 3)
	src := h.g.AddObject(mkCard(t, "Name:Watcher\nTypes:Enchantment\nOracle:x\n"), 0)
	hit := []state.Target{{Player: 2, IsPlayer: true}}
	c := &Ctx{Source: src.ID, Controller: 0, Remembered: copyTargets(hit), Captured: copyTargets(hit)}
	if got := playersOf(DefinedSpec(h, c, "Remembered")); len(got) != 1 || got[0].Player != 2 {
		t.Fatalf("precondition: the capture %+v is not in the plain Remembered read", got)
	}
	wantPlayers(t, definedPlayerIDs(h, c, "RememberedPlayer"), nil, "definedPlayerIDs(RememberedPlayer) with only a trigger capture")
}
