package rules

import (
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"testing"
)

// CR 707.2 / 614.12: a token copy of Diregraf Ghoul has the copied "enters
// tapped" replacement, and it applies to the copy's own entry. The copy
// token's entry MoveZone moves an object outside every zone list, so the
// replacement scan never read its own lines and the copy entered untapped
// (Electroduplicate on a Ghoul here; Abyssal Harvester in the FDN census).
func TestTokenCopyOfDiregrafGhoulEntersTapped(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Diregraf Ghoul", "Electroduplicate")
	ghoul := moveByName(t, e, 0, "Diregraf Ghoul", state.ZBattlefield)
	var ed state.ObjID
	for _, z := range []state.Zone{state.ZHand, state.ZLibrary} {
		for _, id := range e.G.Zone(z, 0) {
			if e.G.Obj(id).Face().Name == "Electroduplicate" {
				ed = id
				if z != state.ZHand {
					e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: z, To: state.ZHand})
				}
			}
		}
	}
	addMana(t, e, 0, "RCC")
	submitChoices(t, e, plainCastOption(t, e, ed).Index)
	if d := e.Pending(); d != nil && d.Kind == decision.KTarget {
		answerTargetAsk(t, e, []state.ObjID{ghoul})
	}
	fdnCensusDrain(t, e)
	found := false
	for _, id := range e.G.Zone(state.ZBattlefield, 0) {
		o := e.G.Obj(id)
		if o.IsToken && o.Face() != nil && o.Face().Name == "Diregraf Ghoul" {
			found = true
			t.Logf("token copy %d tapped=%v", id, o.Tapped)
			if !o.Tapped {
				t.Errorf("token copy of Diregraf Ghoul entered untapped")
			}
		}
	}
	if !found {
		t.Fatal("no token copy created")
	}
}
