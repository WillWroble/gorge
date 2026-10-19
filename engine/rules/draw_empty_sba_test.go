package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

func countDrawEmpty(e *Engine, p state.PlayerID, amount int32) int {
	n := 0
	for _, ev := range e.L.Events {
		if ev.Kind == events.DrawEmpty && ev.Player == p && ev.Amount == amount {
			n++
		}
	}
	return n
}

// TestEmptyLibraryDrawLosesAtTheNextStateBasedCheck pins CR 121.4 and
// 704.5b: an attempted draw from an empty library does not end the game on
// the spot; it marks the player, and the loss is the next state-based check's.
// A second attempt before that check adds no second mark.
func TestEmptyLibraryDrawLosesAtTheNextStateBasedCheck(t *testing.T) {
	t.Parallel()
	e, cfg, _, _ := cantLoseFixture(t, 8320, "")
	cantLoseEmptyLibrary(t, e, 1)

	effects.DrawFor(e, 1)
	effects.DrawFor(e, 1)
	if e.G.Players[1].Lost {
		t.Fatal("seat 1 lost at the draw itself; CR 704.5b makes it a state-based loss")
	}
	if !e.G.Players[1].DrewFromEmpty {
		t.Fatal("the attempted draw did not mark seat 1")
	}
	if got := countDrawEmpty(e, 1, 0); got != 1 {
		t.Fatalf("draw_empty marks = %d, want 1 (one per check)", got)
	}

	e.checkStateBased()
	if !e.G.Players[1].Lost {
		t.Fatal("seat 1 did not lose at the state-based check after drawing from an empty library")
	}
	if e.G.Players[1].DrewFromEmpty {
		t.Fatal("the PlayerLost fold left the empty-draw mark set")
	}
	if !e.G.Over || e.G.Winner != 0 {
		t.Fatalf("game over=%v winner=%d, want seat 0 to win", e.G.Over, e.G.Winner)
	}
	replayCheck(t, e, cfg)
}

// TestEmptyLibraryDrawIsCheckedWhenTheCheckHappens covers the two
// replacement timings CR 704.5b separates: the GameLoss replacement is read
// at the state-based check, not at the draw (Herald leaving between the draw
// and the check does not save the drawer), and the condition is one-shot
// ("since the last time state-based actions were checked"), so a check the
// replacement stopped consumes the mark.
func TestEmptyLibraryDrawIsCheckedWhenTheCheckHappens(t *testing.T) {
	t.Parallel()
	t.Run("herald leaves before the check", func(t *testing.T) {
		e, cfg, hid, _ := cantLoseFixture(t, 8321, "")
		heraldOnBattlefield(t, e, hid)
		cantLoseEmptyLibrary(t, e, 0)
		effects.DrawFor(e, 0)
		e.emit(events.Event{Kind: events.MoveZone, Obj: hid, From: state.ZBattlefield, To: state.ZGraveyard})
		e.checkStateBased()
		if !e.G.Players[0].Lost {
			t.Fatal("Herald had left by the state-based check, yet seat 0 did not lose")
		}
		replayCheck(t, e, cfg)
	})
	t.Run("herald stops the check", func(t *testing.T) {
		e, cfg, hid, _ := cantLoseFixture(t, 8322, "")
		heraldOnBattlefield(t, e, hid)
		cantLoseEmptyLibrary(t, e, 0)
		effects.DrawFor(e, 0)
		e.checkStateBased()
		if e.G.Players[0].Lost {
			t.Fatal("Herald did not stop the empty-library loss")
		}
		if e.G.Players[0].DrewFromEmpty || countDrawEmpty(e, 0, 1) != 1 {
			t.Fatal("the stopped check did not consume the empty-draw mark")
		}
		e.emit(events.Event{Kind: events.MoveZone, Obj: hid, From: state.ZBattlefield, To: state.ZGraveyard})
		e.checkStateBased()
		if e.G.Players[0].Lost {
			t.Fatal("seat 0 lost to a draw attempted before the last state-based check")
		}
		replayCheck(t, e, cfg)
	})
}
