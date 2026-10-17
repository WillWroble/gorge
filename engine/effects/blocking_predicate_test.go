package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// TestBlockingPredicateReadsBlockerRecord pins the reader half of CR
// 506.4/509.1h: the `blocking` filter predicate (engine/effects/filter.go's
// isBlocking) must read the blocker's own record, not only a live attacker's
// BlockedBy. Norin, Swift Survivalist exiles a blocked attacker, so after the
// attacker is gone no BlockedBy names the blocker -- a predicate that still
// scanned only attackers' lists would report it not blocking.
func TestBlockingPredicateReadsBlockerRecord(t *testing.T) {
	g, ids := board(t)
	att := g.Obj(ids["theirBig"])
	blk := g.Obj(ids["myBear"])
	att.IsAttacking = true
	att.Attacking = 0
	l := events.NewLog(1)
	events.Emit(g, l, events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{att.ID, blk.ID}}})

	// Precondition: the pair is really recorded on both objects, so the
	// assertions below are not vacuously true on an unblocked board.
	if !blk.IsBlocking || len(att.BlockedBy) != 1 || att.BlockedBy[0] != blk.ID {
		t.Fatalf("precondition: blocker IsBlocking=%v attacker BlockedBy=%v", blk.IsBlocking, att.BlockedBy)
	}
	if !MatchesSpec(g, "Creature.blocking", blk.ID, 0) {
		t.Fatal("blocking predicate false while the attacker is still blocked")
	}
	// A creature that never blocked must stay false: the predicate is not
	// vacuously true for every creature.
	if MatchesSpec(g, "Creature.blocking", ids["myFlier"], 0) {
		t.Fatal("blocking predicate true for a creature that never blocked")
	}

	// The attacker leaves the battlefield. Its BlockedBy is cleared, so the
	// scan below would find nothing; only the blocker's own IsBlocking record
	// keeps the predicate true.
	events.Emit(g, l, events.Event{Kind: events.MoveZone, Obj: att.ID, From: state.ZBattlefield, To: state.ZExile})
	if len(g.Obj(att.ID).BlockedBy) != 0 {
		t.Fatalf("attacker BlockedBy = %v after leaving, want cleared", g.Obj(att.ID).BlockedBy)
	}
	if !MatchesSpec(g, "Creature.blocking", blk.ID, 0) {
		t.Fatal("blocking predicate false after the attacker left; the blocker's record was not read")
	}
}
