package effects

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/state"
)

// This file is the effects tier's read of the CR 709.5 Room characteristics
// rule -- name, mana value and rules text are those of the unlocked doors.
// The rules tier's chars.RoomName/RoomText are the display-string twins
// (Compute uses them); effects cannot import rules/chars (chars imports
// effects), so the designation gate lives here too. The door bits themselves
// are read through state.Object.DoorUnlocked, the one home that folds the
// cast door, the alternate door and the LockedDoors override; neither tier
// re-reads those fields.

// roomFaces returns o's two printed faces when o is a battlefield,
// non-face-down permanent carrying a two-face card whose faces are both
// Rooms (cards.Face.IsRoom). The rule is a pair of static abilities of the
// permanent, so it functions only on the battlefield; a Room card in any
// other zone keeps its printed characteristics (CR 709.4).
func roomFaces(o *state.Object) ([]*cards.Face, bool) {
	if o == nil || o.Card == nil || len(o.Card.Faces) != 2 || o.Zone != state.ZBattlefield || o.FaceDown {
		return nil, false
	}
	faces := o.Card.Faces
	if faces[0] == nil || faces[1] == nil || !faces[0].IsRoom() || !faces[1].IsRoom() {
		return nil, false
	}
	return faces, true
}

// roomNames is the CR 709.5 name set of a battlefield Room permanent: the
// individual name of each unlocked door (CR 709.4a: an object with two names
// has a chosen name if one of them is it). It is what filter name comparisons
// read -- deliberately NOT the "Left // Right" display string chars.RoomName
// returns, which no effect may choose as a card name. ok is false when o is
// not a battlefield Room; ok true with an empty (or nil) set means the
// permanent has no name (every door locked).
func roomNames(o *state.Object) ([]string, bool) {
	faces, ok := roomFaces(o)
	if !ok {
		return nil, false
	}
	var names []string
	for fi, f := range faces {
		if o.DoorUnlocked(fi) && f.Name != "" {
			names = append(names, f.Name)
		}
	}
	return names, true
}

// roomManaValue is the CR 709.5 mana value of a battlefield Room permanent:
// the sum of the printed mana values of its unlocked doors. ok is false for
// every other object, which then reads its face's printed value -- a Room
// card in any other zone keeps its printed combined characteristics (CR
// 709.4), which the printed reads already give it.
func roomManaValue(o *state.Object) (int, bool) {
	faces, ok := roomFaces(o)
	if !ok {
		return 0, false
	}
	n := 0
	for fi, f := range faces {
		if o.DoorUnlocked(fi) {
			n += int(f.Cmc())
		}
	}
	return n, true
}
