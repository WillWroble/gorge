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
	// A reselect (ChangeCombatants: Misleading Signpost, Portal Mage,
	// Windshaper Planetar) re-points an already-blocked attacker. The
	// re-pointed attack is unblocked, so the former blocker's own record must
	// clear with the attacker's BlockedBy, or the two diverge (the attacker
	// reads unblocked while the blocker still reads blocking).
	t.Run("attacker retargeted", func(t *testing.T) {
		g := state.NewGame([]string{"a", "b", "c"})
		l := NewLog(1)
		bear := bearCard()
		att := g.AddObject(bear, 0).ID
		blk := g.AddObject(bear, 1).ID
		g.SetZone(state.ZLibrary, 0, []state.ObjID{att})
		g.SetZone(state.ZLibrary, 1, []state.ObjID{blk})
		for _, id := range []state.ObjID{att, blk} {
			Emit(g, l, Event{Kind: MoveZone, Obj: id, From: state.ZLibrary, To: state.ZBattlefield})
		}
		g.Obj(att).IsAttacking = true
		g.Obj(att).Attacking = 1
		Emit(g, l, Event{Kind: DeclareBlockers, Pairs: [][2]state.ObjID{{att, blk}}})
		if !g.Obj(blk).IsBlocking || len(g.Obj(att).BlockedBy) != 1 {
			t.Fatalf("precondition: blocker IsBlocking=%v attacker BlockedBy=%v",
				g.Obj(blk).IsBlocking, g.Obj(att).BlockedBy)
		}
		// Seat 2 is the new defender (CombatRetarget's Player is the NEW
		// defender, Obj the attacker).
		Emit(g, l, Event{Kind: CombatRetarget, Obj: att, Player: 2})
		if g.Obj(blk).IsBlocking {
			t.Fatal("former blocker still blocking after the attacker was retargeted")
		}
		if o := g.Obj(att); o.Attacking != 2 || len(o.BlockedBy) != 0 {
			t.Fatalf("retarget: Attacking=%d BlockedBy=%v, want 2 and empty", o.Attacking, o.BlockedBy)
		}
	})
}
