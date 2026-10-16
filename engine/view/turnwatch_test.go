package view

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// TestPlayerViewCarriesTurnWatchersToEveryViewer folds life events through
// events.Apply and checks each seat's tallies are projected, public, to both
// viewers, then cleared by a TurnChange.
func TestPlayerViewCarriesTurnWatchersToEveryViewer(t *testing.T) {
	g := state.NewGame([]string{"a", "b"})
	events.Apply(g, events.Event{Kind: events.LifeChange, Player: 0, Amount: 3})
	events.Apply(g, events.Event{Kind: events.Damage, Player: 1, Amount: 4})
	events.Apply(g, events.Event{Kind: events.PutOnStack, Player: 1, Obj: 99})

	for viewer := state.PlayerID(0); viewer < 2; viewer++ {
		v := Project(g, flatChars{g}, viewer, nil)
		if got := v.Players[0].LifeGainedThisTurn; got != 3 {
			t.Errorf("viewer %d: seat 0 life gained = %d, want 3", viewer, got)
		}
		if got := v.Players[1].LifeLostThisTurn; got != 4 {
			t.Errorf("viewer %d: seat 1 life lost = %d, want 4", viewer, got)
		}
		if got := v.Players[1].SpellsCastThisTurn; got != 1 {
			t.Errorf("viewer %d: seat 1 spells cast = %d, want 1", viewer, got)
		}
	}
	events.Apply(g, events.Event{Kind: events.TurnChange, Player: 1, Amount: 2})
	v := Project(g, flatChars{g}, 0, nil)
	if p := v.Players[0]; p.LifeGainedThisTurn != 0 || v.Players[1].LifeLostThisTurn != 0 || v.Players[1].SpellsCastThisTurn != 0 {
		t.Errorf("watchers survived TurnChange: %+v", v.Players)
	}
}
