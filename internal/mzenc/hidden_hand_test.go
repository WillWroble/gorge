package mzenc

import (
	"testing"

	"github.com/adams-shaun/gorge/view"
)

// A seat (redacted) view carries the opponent's Hand as nil and only HandSize.
// The encoder must emit CardsInHand with that count and nothing that names a
// card.
func TestProcessStateHiddenOpponentHandEmitsCount(t *testing.T) {
	hidden := view.View{Viewer: 0, Players: []view.PlayerView{
		{ID: 0, Life: 20, HandSize: 1, Hand: []view.CardView{{ID: 5, Name: "Island"}}},
		{ID: 1, Life: 20, HandSize: 4, Hand: nil},
	}}
	if hidden.Players[1].Hand != nil {
		t.Fatal("precondition: opponent hand must be redacted (nil)")
	}
	got := ProcessState(hidden, nil, 0, 0, "x")

	withCount := func(n int) map[int32]struct{} {
		return idsFor(func(f *Node) {
			f.SubFeatures("Opponent", true).AddNumericFeature("CardsInHand", n, true)
		})
	}
	for id := range withCount(4) {
		if _, ok := got[id]; !ok {
			t.Fatalf("hidden opponent hand: CardsInHand=4 id %d missing", id)
		}
	}
	// the count is the real count: the thermometer for 5 must not be fully
	// present when the hand holds 4.
	missing := false
	for id := range withCount(5) {
		if _, ok := got[id]; !ok {
			missing = true
		}
	}
	if !missing {
		t.Fatal("CardsInHand=5 ids all present: count is not encoded")
	}

	// Contrast: a visible opponent hand is walked, its card named, and no
	// CardsInHand count is emitted for it; the hidden view names no card.
	visible := view.View{Viewer: 0, Players: []view.PlayerView{
		{ID: 0, Life: 20, HandSize: 0, Hand: []view.CardView{}},
		{ID: 1, Life: 20, HandSize: 1, Hand: []view.CardView{{ID: 9, Name: "Zzyzx Probe"}}},
	}}
	hid := view.View{Viewer: 0, Players: []view.PlayerView{
		{ID: 0, Life: 20, HandSize: 0, Hand: []view.CardView{}},
		{ID: 1, Life: 20, HandSize: 1},
	}}
	named := idsFor(func(f *Node) {
		f.SubFeatures("Opponent", true).SubFeatures("Hand", true).SubFeatures("Zzyzx Probe", true)
	})
	// the card's own ids: everything beyond the shared Opponent/Hand prefix.
	prefix := idsFor(func(f *Node) {
		f.SubFeatures("Opponent", true).SubFeatures("Hand", true)
	})
	for id := range prefix {
		delete(named, id)
	}
	if len(named) == 0 {
		t.Fatal("precondition: card contributes no ids of its own")
	}
	vis, hide := ProcessState(visible, nil, 0, 0, "x"), ProcessState(hid, nil, 0, 0, "x")
	for id := range named {
		if _, ok := vis[id]; !ok {
			t.Fatalf("precondition: visible hand did not emit card id %d", id)
		}
		if _, ok := hide[id]; ok {
			t.Fatalf("hidden hand leaked card-subtree id %d", id)
		}
	}
	cnt := idsFor(func(f *Node) {
		f.SubFeatures("Opponent", true).AddNumericFeature("CardsInHand", 1, true)
	})
	for id := range cnt {
		if _, ok := hide[id]; !ok {
			t.Fatalf("hidden hand: CardsInHand=1 id %d missing", id)
		}
	}
}
