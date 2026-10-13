package chars

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/state"
)

func roomTextCard() *cards.Card {
	return &cards.Card{AlternateMode: "Split", Faces: []*cards.Face{
		{Name: "Left", Types: []string{"Enchantment", "Room"}, Oracle: "left rules"},
		{Name: "Right", Types: []string{"Enchantment", "Room"}, Oracle: "right rules"},
	}}
}

// TestRoomTextByDoorState pins the CR 709.5 rules-text half: a Room
// permanent's text is its unlocked doors' printed Oracle text, joined with a
// newline when both are unlocked and empty when neither is. ok is false under
// exactly RoomName's conditions (not a two-face Room card).
func TestRoomTextByDoorState(t *testing.T) {
	nonRoom := &cards.Card{AlternateMode: "Split", Faces: []*cards.Face{
		{Name: "Left", Types: []string{"Enchantment"}, Oracle: "left rules"},
		{Name: "Right", Types: []string{"Sorcery"}, Oracle: "right rules"},
	}}
	singleFace := &cards.Card{Faces: []*cards.Face{
		{Name: "Solo", Types: []string{"Enchantment", "Room"}, Oracle: "solo rules"},
	}}

	cases := []struct {
		name     string
		obj      state.Object
		wantL    bool
		wantR    bool
		wantText string
		wantOK   bool
	}{
		{"both locked", state.Object{Card: roomTextCard(), FaceIdx: 0}, false, false, "", true},
		{"front unlocked", state.Object{Card: roomTextCard(), FaceIdx: 0, CastDoor: true}, true, false, "left rules", true},
		{"back unlocked", state.Object{Card: roomTextCard(), FaceIdx: 0, Unlocked: true}, false, true, "right rules", true},
		{"both unlocked", state.Object{Card: roomTextCard(), FaceIdx: 0, CastDoor: true, Unlocked: true}, true, true, "left rules\nright rules", true},
		{"cast door re-locked", state.Object{Card: roomTextCard(), FaceIdx: 0, CastDoor: true, Unlocked: true, LockedDoors: 1 << 0}, false, true, "right rules", true},
		{"non-room split card", state.Object{Card: nonRoom, FaceIdx: 0, CastDoor: true}, false, false, "", false},
		{"single-face card", state.Object{Card: singleFace, FaceIdx: 0, CastDoor: true}, true, false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Precondition: the door designation the rule reads is what this
			// case claims. A zero-valued Object would otherwise make the text
			// assertion below vacuous.
			if got := tc.obj.DoorUnlocked(0); got != tc.wantL {
				t.Fatalf("precondition: DoorUnlocked(0)=%v, want %v", got, tc.wantL)
			}
			if got := tc.obj.DoorUnlocked(1); got != tc.wantR {
				t.Fatalf("precondition: DoorUnlocked(1)=%v, want %v", got, tc.wantR)
			}
			text, ok := RoomText(&tc.obj)
			if ok != tc.wantOK || text != tc.wantText {
				t.Fatalf("RoomText=%q,%v, want %q,%v", text, ok, tc.wantText, tc.wantOK)
			}
			// The name and text halves must agree on which objects the rule
			// governs: a caller that gates on RoomName's ok cannot be handed a
			// text answer for an object outside the rule.
			if _, nameOK := RoomName(&tc.obj); nameOK != ok {
				t.Fatalf("RoomName ok=%v disagrees with RoomText ok=%v", nameOK, ok)
			}
		})
	}
}
