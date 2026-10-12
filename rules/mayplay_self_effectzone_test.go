package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// A card's own Continuous MayPlay$ static functions off the battlefield only
// when the script names its EffectZone$ (CR 113.6; Forge's default is the
// battlefield). Omniscience in hand used to offer its own free cast through
// the alternative-cost walk, and a top-of-library grant (Future Sight) let the
// card play itself from the top of its library through mayPlayGrant's
// self-grant pass.

// TestOmniscienceInHandGrantsNothing: in hand, Omniscience offers no free cast
// of itself or of another hand card; on the battlefield it still does.
func TestOmniscienceInHandGrantsNothing(t *testing.T) {
	t.Parallel()
	e := handEngine(t, corpusAlternativeCard(t, "Omniscience"), corpusAlternativeCard(t, "Craw Wurm"))
	omni, wurm := e.G.Zone(state.ZHand, 0)[0], e.G.Zone(state.ZHand, 0)[1]
	if _, ok := freeHandCastOption(e, 0, omni); ok {
		t.Errorf("Omniscience in hand offers its own free cast")
	}
	if _, ok := freeHandCastOption(e, 0, wurm); ok {
		t.Errorf("Omniscience in hand offers Craw Wurm a free cast")
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: omni, From: state.ZHand, To: state.ZBattlefield})
	if _, ok := freeHandCastOption(e, 0, wurm); !ok {
		t.Errorf("Omniscience on the battlefield offers Craw Wurm no free cast")
	}
}

// TestFutureSightOnTopOfLibraryGrantsNothing: Future Sight on top of its
// owner's library does not let that player play it from there; on the
// battlefield it grants the new top card.
func TestFutureSightOnTopOfLibraryGrantsNothing(t *testing.T) {
	t.Parallel()
	e := handEngine(t)
	fs := e.G.AddObject(corpusAlternativeCard(t, "Future Sight"), 0)
	fs.Zone = state.ZLibrary
	e.G.SetZone(state.ZLibrary, 0, append([]state.ObjID{fs.ID}, e.G.Zone(state.ZLibrary, 0)...))
	if _, ok := e.mayPlayGrant(0, fs.ID); ok {
		t.Errorf("Future Sight on top of the library grants itself a play permission")
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: fs.ID, From: state.ZLibrary, To: state.ZBattlefield})
	top := e.G.Zone(state.ZLibrary, 0)[0]
	if _, ok := e.mayPlayGrant(0, top); !ok {
		t.Errorf("Future Sight on the battlefield grants the top card of the library nothing")
	}
}
