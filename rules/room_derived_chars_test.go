package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
)

// roomDerivedSrc is a synthetic two-door Room with distinct mana costs and
// Oracle texts per half, so a Derived read can tell which doors it combined.
const roomDerivedSrc = "Name:Front Room\nManaCost:1 U\nTypes:Enchantment Room\nOracle:front rules\n" +
	"ALTERNATE\nName:Back Room\nManaCost:2 U\nTypes:Enchantment Room\nOracle:back rules\nAlternateMode:Split\n"

// TestDerivedRoomNameTextAndRenameFollowDoors is the engine-level CR 709.5
// gate: Derived(id).Name/.Text/.NameSet follow the door designations (none,
// one or both unlocked; the empty string is a BOUND name for a both-locked
// Room), a DoorLock re-narrows them, and a layer-3 SetName$ still overrides
// the name while leaving the rules text alone. The rename table carries the
// override as the object's one effective name, and carries nothing for a Room
// with no override (its name characteristics are the doors' individual names,
// which the effects tier reads itself).
func TestDerivedRoomNameTextAndRenameFollowDoors(t *testing.T) {
	e := layerEngine(t)
	id := onBoardCard(t, e, 0, card(t, roomDerivedSrc))
	o := e.G.Obj(id)

	if o.DoorUnlocked(0) || o.DoorUnlocked(1) {
		t.Fatalf("precondition: a directly placed Room has unlocked doors: face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	d := e.Derived(id)
	if d.Name != "" || !d.NameSet {
		t.Fatalf("both-locked Derived name = %q set=%v, want \"\" true", d.Name, d.NameSet)
	}
	if d.Text != "" {
		t.Fatalf("both-locked Derived text = %q, want the empty string", d.Text)
	}

	// Cast the back door: only it is unlocked.
	e.emit(events.Event{Kind: events.DoorUnlock, Obj: id, Amount: 2})
	if !o.DoorUnlocked(1) || o.DoorUnlocked(0) {
		t.Fatalf("precondition: back-door unlock left face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	d = e.Derived(id)
	if d.Name != "Back Room" || !d.NameSet {
		t.Fatalf("back-door Derived name = %q set=%v, want %q true", d.Name, d.NameSet, "Back Room")
	}
	if d.Text != "back rules" {
		t.Fatalf("back-door Derived text = %q, want %q", d.Text, "back rules")
	}

	// Unlock the front door too: both names, both texts.
	e.emit(events.Event{Kind: events.DoorUnlock, Obj: id, Amount: 1})
	if !o.DoorUnlocked(0) || !o.DoorUnlocked(1) {
		t.Fatalf("precondition: front-door unlock left face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	d = e.Derived(id)
	if d.Name != "Front Room // Back Room" || !d.NameSet {
		t.Fatalf("both-door Derived name = %q set=%v, want %q true", d.Name, d.NameSet, "Front Room // Back Room")
	}
	if d.Text != "front rules\nback rules" {
		t.Fatalf("both-door Derived text = %q, want %q", d.Text, "front rules\nback rules")
	}

	// Re-lock the back door (Keys to the House): the name narrows again.
	e.emit(events.Event{Kind: events.DoorLock, Obj: id, Amount: 2})
	if o.DoorUnlocked(1) || !o.DoorUnlocked(0) {
		t.Fatalf("precondition: back-door lock left face0=%v face1=%v", o.DoorUnlocked(0), o.DoorUnlocked(1))
	}
	d = e.Derived(id)
	if d.Name != "Front Room" || !d.NameSet {
		t.Fatalf("re-locked Derived name = %q set=%v, want %q true", d.Name, d.NameSet, "Front Room")
	}
	if d.Text != "front rules" {
		t.Fatalf("re-locked Derived text = %q, want %q", d.Text, "front rules")
	}

	// No SetName$ is active, so the rename table must carry nothing for the
	// Room: the effects tier answers its name comparisons from the doors.
	e.refreshRenames()
	for _, ent := range e.EffectiveNames() {
		if ent.ID == id {
			t.Fatalf("rename table carries a Room entry without a SetName$: %+v", ent)
		}
	}

	// A layer-3 SetName$ overrides the Room rule for the name only, and the
	// rename table carries it.
	e.AddContinuous(ContinuousEffect{Source: id, Controller: 0, Affects: "Card.Self", Layer: LText, SetName: "Renamed Room"})
	d = e.Derived(id)
	if d.Name != "Renamed Room" || !d.NameSet {
		t.Fatalf("SetName$ Derived name = %q set=%v, want %q true", d.Name, d.NameSet, "Renamed Room")
	}
	if d.Text != "front rules" {
		t.Fatalf("SetName$ Derived text = %q, want the door text %q", d.Text, "front rules")
	}
	e.refreshRenames()
	found := false
	for _, ent := range e.EffectiveNames() {
		if ent.ID == id {
			found = true
			if ent.Name != "Renamed Room" {
				t.Fatalf("rename table entry = %q, want %q", ent.Name, "Renamed Room")
			}
		}
	}
	if !found {
		t.Fatalf("rename table %v does not carry the SetName$ override for obj %d", e.EffectiveNames(), id)
	}
}
