package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// TestChangeZoneAllGainControlEntersUnderNewController pins CR 110.2a /
// 603.3a: Rise of the Dark Realms' "put all creature cards from all
// graveyards onto the battlefield under your control" folds the new
// controller BEFORE the battlefield entry, so the entering permanent's ETB
// trigger (collected from the MoveZone) is controlled by the resolving
// controller, not by the card's owner.
func TestChangeZoneAllGainControlEntersUnderNewController(t *testing.T) {
	g, ids := board(t)
	h := &fakeHost{g: g}
	big := g.Obj(ids["theirBig"])
	if big == nil || big.Owner != 1 || big.Zone != state.ZBattlefield {
		t.Fatalf("precondition: theirBig = %#v, want seat 1's battlefield creature", big)
	}
	// Put the opponent's creature into its owner's graveyard first.
	h.Emit(events.Event{Kind: events.MoveZone, Obj: big.ID, From: state.ZBattlefield, To: state.ZGraveyard})
	if big.Zone != state.ZGraveyard || big.Controller != 1 {
		t.Fatalf("precondition: creature zone=%s controller=%d, want graveyard/1", big.Zone, big.Controller)
	}
	h.log = nil

	ability := sa(t, "DB$ ChangeZoneAll | ChangeType$ Creature | Origin$ Graveyard | Destination$ Battlefield | GainControl$ True")
	if ability.API != "ChangeZoneAll" {
		t.Fatalf("precondition: API = %q, want ChangeZoneAll", ability.API)
	}
	Resolve(h, &Ctx{Source: ids["myLand"], Controller: 0}, ability)

	if big.Zone != state.ZBattlefield || big.Controller != 0 {
		t.Fatalf("creature zone=%s controller=%d, want battlefield under seat 0", big.Zone, big.Controller)
	}
	moveIdx, controlIdx := -1, -1
	for i, ev := range h.log {
		switch {
		case ev.Kind == events.MoveZone && ev.Obj == big.ID:
			moveIdx = i
		case ev.Kind == events.ControlChange && ev.Obj == big.ID:
			controlIdx = i
		}
	}
	if moveIdx < 0 || controlIdx < 0 {
		t.Fatalf("want a MoveZone and a ControlChange for the creature, got move=%d control=%d: %+v", moveIdx, controlIdx, h.log)
	}
	if controlIdx > moveIdx {
		t.Fatalf("ControlChange (%d) followed the battlefield MoveZone (%d): the ETB trigger would be seat 1's", controlIdx, moveIdx)
	}
}
