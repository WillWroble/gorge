package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// TestExiledWithAmountCountsForwardLinkEverySeat pins the `ExiledWith$`
// count head on the shared exiled-with reader (exiledWithSet): Veteran
// Survivor's real Survival exile, run through effChangeZone, takes a card
// from an OPPONENT's graveyard. The move records only the source's forward
// Object.ExiledCards link (no ExiledWithSource on the face, so no reverse
// stamp) and lands the card in the opponent's exile -- the count must still
// see it (CR 406.6 / 607.2a). Once the card leaves exile it is a new object
// (CR 400.7) and drops out of the count.
func TestExiledWithAmountCountsForwardLinkEverySeat(t *testing.T) {
	survivor, exile := corpusSA(t, "Veteran Survivor", "TrigChangeZone")
	if exile.API != "ChangeZone" || exile.Params["Destination"] != "Exile" || exile.Params["Origin"] != "Graveyard" {
		t.Fatalf("Veteran Survivor exile fixture changed: %+v", exile)
	}
	h := newHost(t, 2)
	src := h.g.AddObject(survivor, 0)
	h.Emit(events.Event{Kind: events.MoveZone, Obj: src.ID, From: state.ZLibrary, To: state.ZBattlefield})
	victim := h.g.AddObject(mkCard(t, "Name:Victim\nTypes:Creature\nPT:1/1\nOracle:x\n"), 1)
	h.Emit(events.Event{Kind: events.MoveZone, Obj: victim.ID, From: state.ZLibrary, To: state.ZGraveyard})
	c := &Ctx{Source: src.ID, Controller: 0, SVars: survivor.Faces[0].SVars, Targets: []state.Target{{Obj: victim.ID}}}
	effChangeZone(h, c, exile)
	v := h.g.Obj(victim.ID)
	if v.Zone != state.ZExile || v.ExiledWith == src.ID || len(h.g.Obj(src.ID).ExiledCards) != 1 {
		t.Fatalf("precondition: victim zone %v ExiledWith %d, source ExiledCards %v -- want exile via the forward link only",
			v.Zone, v.ExiledWith, h.g.Obj(src.ID).ExiledCards)
	}
	read := &Ctx{Source: src.ID, Controller: 0}
	if got, ok := EvalCountOK(h, read, "ExiledWith$Amount"); !ok || got != 1 {
		t.Fatalf("ExiledWith$Amount = %d (ok=%v), want 1", got, ok)
	}
	h.Emit(events.Event{Kind: events.MoveZone, Obj: victim.ID, From: state.ZExile, To: state.ZGraveyard})
	if got, ok := EvalCountOK(h, read, "ExiledWith$Amount"); !ok || got != 0 {
		t.Fatalf("ExiledWith$Amount after the card left exile = %d (ok=%v), want 0", got, ok)
	}
}
