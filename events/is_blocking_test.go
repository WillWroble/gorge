package events

import (
	"testing"

	"github.com/adams-shaun/gorge/state"
)

// CR 506.4/509.1h: a blocker stays a blocker after the attacker it blocked
// leaves combat (Norin, Swift Survivalist exiles a blocked attacker). The
// record lives on the blocker, and ends with combat or when it leaves.
func TestIsBlockingSurvivesAttackerLeaving(t *testing.T) {
	setup := func(t *testing.T) (*state.Game, *Log, state.ObjID, state.ObjID) {
		g, l := twoPlayer(t)
		att := g.Zone(state.ZLibrary, 0)[0]
		blk := g.Zone(state.ZLibrary, 1)[0]
		for _, id := range []state.ObjID{att, blk} {
			Emit(g, l, Event{Kind: MoveZone, Obj: id, From: state.ZLibrary, To: state.ZBattlefield})
		}
		g.Obj(att).IsAttacking = true
		Emit(g, l, Event{Kind: DeclareBlockers, Pairs: [][2]state.ObjID{{att, blk}}})
		if !g.Obj(blk).IsBlocking || g.Obj(att).IsBlocking {
			t.Fatalf("precondition: blocker IsBlocking=%v attacker IsBlocking=%v", g.Obj(blk).IsBlocking, g.Obj(att).IsBlocking)
		}
		return g, l, att, blk
	}
	t.Run("attacker exiled", func(t *testing.T) {
		g, l, att, blk := setup(t)
		Emit(g, l, Event{Kind: MoveZone, Obj: att, From: state.ZBattlefield, To: state.ZExile})
		if !g.Obj(blk).IsBlocking {
			t.Fatal("blocker stopped blocking when its attacker left")
		}
		if g.Obj(att).IsBlocking {
			t.Fatal("departed attacker marked blocking")
		}
	})
	t.Run("blocker leaves", func(t *testing.T) {
		g, l, _, blk := setup(t)
		Emit(g, l, Event{Kind: MoveZone, Obj: blk, From: state.ZBattlefield, To: state.ZGraveyard})
		if g.Obj(blk).IsBlocking {
			t.Fatal("blocker still blocking after leaving the battlefield")
		}
	})
	t.Run("end of combat", func(t *testing.T) {
		g, l, _, blk := setup(t)
		Emit(g, l, Event{Kind: EndCombatReset})
		if g.Obj(blk).IsBlocking {
			t.Fatal("blocker still blocking after EndCombatReset")
		}
	})
	t.Run("removed from combat", func(t *testing.T) {
		g, l, _, blk := setup(t)
		Emit(g, l, Event{Kind: EndCombatReset, Obj: blk})
		if g.Obj(blk).IsBlocking {
			t.Fatal("removed blocker still blocking")
		}
	})
}
