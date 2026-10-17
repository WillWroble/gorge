package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

const testEdgarScript = "Name:Test Edgar\nTypes:Creature Human\nPT:2/2\nS:Mode$ FlipCoinMod | ValidPlayer$ You | CheckSVar$ Count$YouFlipThisTurn | SVarCompare$ EQ0 | Result$ True\nOracle:x\n"

// tailsHost answers every RNG draw with 1 (tails / a lost flip), so a
// flip-until-you-lose loop terminates on its first unforced flip.
type tailsHost struct{ *fakeHost }

func (h tailsHost) Rand(int) int { h.n++; return 1 }

func edgarHost(t *testing.T) (*fakeHost, *state.Object) {
	t.Helper()
	h := newHost(t, 2)
	obj := h.g.AddObject(mkCard(t, testEdgarScript), 0)
	obj.Zone = state.ZBattlefield
	h.g.SetZone(state.ZBattlefield, 0, []state.ObjID{obj.ID})
	if h.g.Players[0].CoinFlipsThisTurn != 0 {
		t.Fatal("precondition: player already flipped")
	}
	return h, obj
}

// TestFlipCoinModFirstBatchForcedThenOrdinary pins Edgar, King of Figaro's
// Oracle text ("The first time you flip one or more coins each turn, those
// coins come up heads and you win those flips", CR 705.1-705.3): an Amount$ 2
// flip is ONE flipping instruction, so both of its coins are forced heads with
// no RNG draw; the turn's next instruction is random again; the tally resets
// at the turn change.
func TestFlipCoinModFirstBatchForcedThenOrdinary(t *testing.T) {
	h, obj := edgarHost(t)
	c := &Ctx{Source: obj.ID, Controller: 0}
	Resolve(h, c, sa(t, "DB$ FlipCoin | Amount$ 2 | RememberResult$ True"))
	if c.FlipMemory == nil || len(c.FlipMemory.Results) != 2 {
		t.Fatalf("precondition: expected two actual flips, got %+v", c.FlipMemory)
	}
	if !c.FlipMemory.Results[0].Heads || !c.FlipMemory.Results[1].Heads || h.n != 0 {
		t.Fatalf("both coins of the first batch should be forced heads: results=%v draws=%d", c.FlipMemory.Results, h.n)
	}
	if h.g.Players[0].CoinFlipsThisTurn != 2 {
		t.Fatalf("flip tally = %d, want 2", h.g.Players[0].CoinFlipsThisTurn)
	}
	if clone := h.g.Clone(); clone.Players[0].CoinFlipsThisTurn != 2 {
		t.Fatalf("clone lost flip tally: %d", clone.Players[0].CoinFlipsThisTurn)
	}
	// The second instruction this turn is not the first: the RNG decides.
	c2 := &Ctx{Source: obj.ID, Controller: 0}
	Resolve(h, c2, sa(t, "DB$ FlipCoin | RememberResult$ True"))
	if h.n != 1 {
		t.Fatalf("second flip instruction consumed %d draws, want 1", h.n)
	}
	h.Emit(events.Event{Kind: events.TurnChange, Player: 1, Amount: h.g.Turn + 1})
	if h.g.Players[0].CoinFlipsThisTurn != 0 {
		t.Fatal("flip tally survived turn change")
	}
}

// TestFlipCoinModFlipUntilLoseForcesOnlyFirstFlip: "flip a coin until you
// lose a flip" is a sequence of single flips, so Edgar wins the first one and
// every later flip is the seeded RNG's (here: tails, which ends the loop).
func TestFlipCoinModFlipUntilLoseForcesOnlyFirstFlip(t *testing.T) {
	fh, obj := edgarHost(t)
	h := tailsHost{fh}
	c := &Ctx{Source: obj.ID, Controller: 0}
	Resolve(h, c, sa(t, "DB$ FlipCoin | FlipUntilYouLose$ True | RememberResult$ True"))
	if c.FlipMemory == nil || len(c.FlipMemory.Results) != 2 {
		t.Fatalf("want a forced win then one random loss, got %+v", c.FlipMemory)
	}
	if !c.FlipMemory.Results[0].Heads || c.FlipMemory.Results[1].Heads || fh.n != 1 {
		t.Fatalf("results=%v draws=%d, want [heads tails] with one draw", c.FlipMemory.Results, fh.n)
	}
}
