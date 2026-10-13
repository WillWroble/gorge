package view_test

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/state"
	"github.com/adams-shaun/gorge/view"
)

const viewRoomSrc = "Name:View Room\nManaCost:1 U\nTypes:Enchantment Room\nOracle:front rules\n" +
	"ALTERNATE\nName:View Chamber\nManaCost:2 U\nTypes:Enchantment Room\nOracle:back rules\nAlternateMode:Split\n"

// TestProjectionRoomNameFollowsDoors pins the wire-visible CR 709.5 result:
// a battlefield Room's CardView.Name is its unlocked doors' names (joined
// " // " when both are, the EMPTY string when neither is) while CardName
// keeps naming the printed card the permanent is, for the deck list and the
// own-library composition key.
func TestProjectionRoomNameFollowsDoors(t *testing.T) {
	c, diags := cards.ParseBytes("view-room.txt", []byte(viewRoomSrc))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	c.Link()
	deck := []*cards.Card{c, c, c, c, c, c, c, c}
	e := rules.New(rules.Config{Seed: 7, Names: []string{"a", "b"}, Decks: [][]*cards.Card{deck, deck}})
	e.Advance()

	var id state.ObjID
	for _, z := range []state.Zone{state.ZHand, state.ZLibrary} {
		for _, cand := range e.G.Zone(z, 0) {
			if o := e.G.Obj(cand); o != nil && o.Face() != nil && o.Face().Name == "View Room" {
				id = cand
				break
			}
		}
		if id != 0 {
			break
		}
	}
	if id == 0 {
		t.Fatal("precondition: no View Room in seat 0's hand or library")
	}
	o := e.G.Obj(id)
	cardView := func() view.CardView {
		t.Helper()
		v := view.ProjectFor(e.G, e, 0, view.Seat, nil)
		for _, pv := range v.Players {
			for _, cv := range pv.Battlefield {
				if cv.ID == id {
					return cv
				}
			}
		}
		t.Fatalf("obj %d not on the projected battlefield: %+v", id, v.Players)
		return view.CardView{}
	}

	// Placed with no door designated: both doors locked, so the permanent has
	// no name at all and CardName names the printed card.
	from := o.Zone
	e.Emit(events.Event{Kind: events.MoveZone, Obj: id, From: from, To: state.ZBattlefield})
	if o.DoorUnlocked(0) || o.DoorUnlocked(1) {
		t.Fatalf("precondition: placement left doors face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	cv := cardView()
	if cv.Name != "" || cv.CardName != "View Room" {
		t.Fatalf("both-locked CardView name=%q card_name=%q, want \"\" and View Room", cv.Name, cv.CardName)
	}

	// Cast the front door through the stack: the cast face is designated.
	e.Emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZBattlefield, To: state.ZStack})
	e.Emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZStack, To: state.ZBattlefield})
	if !o.DoorUnlocked(0) || o.DoorUnlocked(1) {
		t.Fatalf("precondition: cast designation left doors face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	cv = cardView()
	if cv.Name != "View Room" || cv.CardName != "" {
		t.Fatalf("front-door CardView name=%q card_name=%q, want View Room and \"\"", cv.Name, cv.CardName)
	}

	// Paid unlock of the back door: both names, joined.
	e.Emit(events.Event{Kind: events.DoorUnlock, Obj: id, Amount: 2})
	if !o.DoorUnlocked(1) {
		t.Fatalf("precondition: back door did not unlock: face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	cv = cardView()
	if cv.Name != "View Room // View Chamber" || cv.CardName != "View Room" {
		t.Fatalf("both-door CardView name=%q card_name=%q, want the joined name and the printed card", cv.Name, cv.CardName)
	}
}
