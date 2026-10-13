package v2engine

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/state"
)

// TestVisibleNameRoomFollowsDoors is the observation gate for CR 709.5: the
// name the viewer may see for a battlefield Room is its unlocked doors' names
// -- a BOUND empty string when neither door is unlocked, which the printed
// fallback (f.Name) must not paper over -- and the printed front name only
// off the battlefield.
func TestVisibleNameRoomFollowsDoors(t *testing.T) {
	const src = "Name:Front Door\nManaCost:2\nTypes:Enchantment Room\nOracle:front\n" +
		"ALTERNATE\nName:Back Door\nManaCost:3\nTypes:Enchantment Room\nOracle:back\nAlternateMode:Split\n"
	c, diags := cards.ParseBytes("room.txt", []byte(src))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	c.Link()
	deck := []*cards.Card{c, c, c, c, c, c, c, c}
	e := rules.New(rules.Config{Seed: 1, Names: []string{"a", "b"}, Decks: [][]*cards.Card{deck, deck}})
	g := &Game{e: e}
	show := func(name *string) string {
		if name == nil {
			return "<nil>"
		}
		return *name
	}

	id := e.G.AddObject(c, 0).ID
	o := e.G.Obj(id)
	if o.DoorUnlocked(0) || o.DoorUnlocked(1) {
		t.Fatalf("precondition: a directly placed Room has unlocked doors: face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}

	// Off the battlefield (a card in the viewer's own hand): the printed face.
	o.Zone = state.ZHand
	o.Owner = 0
	if name := g.visibleName(o, 0); name == nil || *name != "Front Door" {
		t.Fatalf("hand visibleName = %q, want the printed front name", show(name))
	}

	// On the battlefield with no door designated: the permanent is nameless,
	// and that empty name is the real answer, not a fallback trigger.
	o.Zone = state.ZBattlefield
	name := g.visibleName(o, 0)
	if name == nil || *name != "" {
		t.Fatalf("both-locked visibleName = %q, want a bound empty string", show(name))
	}

	// Cast the front door: only its name is visible.
	e.Emit(events.Event{Kind: events.DoorUnlock, Obj: id, Amount: 1})
	if !o.DoorUnlocked(0) || o.DoorUnlocked(1) {
		t.Fatalf("precondition: front-door unlock left face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	if name = g.visibleName(o, 0); name == nil || *name != "Front Door" {
		t.Fatalf("front-door visibleName = %q, want Front Door", show(name))
	}

	// Both doors: the joined name.
	e.Emit(events.Event{Kind: events.DoorUnlock, Obj: id, Amount: 2})
	if !o.DoorUnlocked(1) {
		t.Fatalf("precondition: back door did not unlock: face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	if name = g.visibleName(o, 0); name == nil || *name != "Front Door // Back Door" {
		t.Fatalf("both-door visibleName = %q, want the joined name", show(name))
	}
}
