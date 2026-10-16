package events

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/state"
)

func walkerCard() *cards.Card {
	c, _ := cards.ParseBytes("w.txt", []byte("Name:Walker\nManaCost:2 U\nTypes:Legendary Planeswalker Jace\nLoyalty:3\nOracle:x\n"))
	c.Link()
	return c
}

// TestAttackWatchersFoldAndReset: a DeclareAttackers at a player sets the
// attacking controller's AttackedPlayersThisTurn bit for that defender, one
// at a planeswalker sets AttackedPlaneswalkersThisTurn for its controller
// (CR 508.1b), a battle attack sets neither, and TurnChange clears both.
func TestAttackWatchersFoldAndReset(t *testing.T) {
	g, l := twoPlayer(t)
	bear := g.AddObject(bearCard(), 1)
	bear.Zone = state.ZBattlefield
	walker := g.AddObject(walkerCard(), 0)
	walker.Zone = state.ZBattlefield
	g.SetZone(state.ZBattlefield, 1, []state.ObjID{bear.ID})
	g.SetZone(state.ZBattlefield, 0, []state.ObjID{walker.ID})

	Emit(g, l, Event{Kind: DeclareAttackers, Player: 0, IDs: []state.ObjID{bear.ID}})
	if got := g.Players[1].AttackedPlayersThisTurn; got != 1 {
		t.Fatalf("player attack bits = %b, want seat 0", got)
	}
	if g.Players[1].AttackedPlaneswalkersThisTurn != 0 || g.Players[0].AttackedPlayersThisTurn != 0 {
		t.Fatal("a player attack set a planeswalker bit or the defender's own watcher")
	}
	Emit(g, l, Event{Kind: DeclareAttackers, Player: 0, Obj: walker.ID, IDs: []state.ObjID{bear.ID}})
	if got := g.Players[1].AttackedPlaneswalkersThisTurn; got != 1 {
		t.Fatalf("planeswalker attack bits = %b, want seat 0", got)
	}
	if c := g.Clone(); c.Players[1].AttackedPlayersThisTurn != 1 || c.Players[1].AttackedPlaneswalkersThisTurn != 1 {
		t.Fatal("clone lost the attack watchers")
	}
	Emit(g, l, Event{Kind: TurnChange, Player: 0, Amount: 2})
	if g.Players[1].AttackedPlayersThisTurn != 0 || g.Players[1].AttackedPlaneswalkersThisTurn != 0 {
		t.Fatal("TurnChange did not clear the attack watchers")
	}
}
