package chars

import (
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/state"
)

// roomFaces returns o's two printed faces when o carries a two-face card
// whose faces are both Rooms (cards.Face.IsRoom), else ok=false. It is the
// designation half of the CR 709.5 helpers below: whether the rule functions
// for o (a battlefield, non-face-down permanent) is the caller's gate, since
// only the caller knows which zone the characteristics are being read for.
func roomFaces(o *state.Object) ([]*cards.Face, bool) {
	if o == nil || o.Card == nil || len(o.Card.Faces) != 2 {
		return nil, false
	}
	faces := o.Card.Faces
	if faces[0] == nil || faces[1] == nil || !faces[0].IsRoom() || !faces[1].IsRoom() {
		return nil, false
	}
	return faces, true
}

// RoomName is the CR 709.5 name of a Room permanent: the name of each door
// that is currently unlocked, joined with " // " when both are, and the empty
// string when neither is. ok is false unless o carries a two-face card whose
// printed faces are both Rooms (cards.Face.IsRoom); every other object is not
// governed by this rule, and the caller falls back to the printed face name.
//
// XMage derives the same string in RoomCharacteristicsEffect: it starts from
// the split card's name ("Left // Right") and strips "Left // " when the left
// door is locked and " // Right" when the right is, leaving "" for a
// both-locked Room. Index 0 is the left (front) face, Forge's face 0.
//
// The door designations are read through state.Object.DoorUnlocked, the one
// home that folds the cast door (CastDoor), the alternate door (Unlocked) and
// the LockedDoors override together; this helper never re-reads those fields.
func RoomName(o *state.Object) (string, bool) {
	faces, ok := roomFaces(o)
	if !ok {
		return "", false
	}
	left, right := o.DoorUnlocked(0), o.DoorUnlocked(1)
	switch {
	case left && right:
		return faces[0].Name + " // " + faces[1].Name, true
	case left:
		return faces[0].Name, true
	case right:
		return faces[1].Name, true
	default:
		return "", true
	}
}

// RoomText is the CR 709.5 rules text of a Room permanent: the printed Oracle
// text of each door that is currently unlocked, joined with a newline when
// both are, and the empty string when neither is. ok is false under exactly
// the conditions RoomName's ok is false; the caller decides whether o is a
// battlefield, non-face-down permanent for which the rule functions. Compute
// reads it for the text seed; the name-only walk has no text output, and a
// layer-3 TextSet/TextFrom applies on top afterwards.
func RoomText(o *state.Object) (string, bool) {
	faces, ok := roomFaces(o)
	if !ok {
		return "", false
	}
	var b strings.Builder
	for fi, f := range faces {
		if !o.DoorUnlocked(fi) {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(f.Oracle)
	}
	return b.String(), true
}

// IsRoomCard reports whether o is a two-faced Enchantment Room card.
func IsRoomCard(o *state.Object) bool {
	if o == nil || o.Card == nil || len(o.Card.Faces) != 2 {
		return false
	}
	f := o.Card.Faces
	return f[0] != nil && f[1] != nil && f[0].IsRoom() && f[1].IsRoom()
}
