package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

const (
	hintStoneRain = "Name:Stone Rain Fixture\nManaCost:2 R\nTypes:Sorcery\nA:SP$ Destroy | ValidTgts$ Land\nOracle:x\n"
	hintBolt      = "Name:Bolt Hint Fixture\nManaCost:R\nTypes:Instant\nA:SP$ DealDamage | ValidTgts$ Any | NumDmg$ 3\nOracle:x\n"
	hintGrowth    = "Name:Growth Hint Fixture\nManaCost:G\nTypes:Instant\nA:SP$ Pump | ValidTgts$ Creature | NumAtt$ +3\nOracle:x\n"
	hintLandSrc   = "Name:Hint Land\nTypes:Land\nOracle:x\n"
)

// hintedCast returns the single cast option for id on the pending priority
// decision after asking for hints.
func hintedCast(t *testing.T, e *Engine, id state.ObjID) decision.Option {
	t.Helper()
	d := e.Pending()
	for _, o := range d.Options {
		if o.Kind == "cast" && o.Obj == id {
			if o.TargetHints != 0 {
				t.Fatal("hint present before EnsureTargetHints: it must be opt-in")
			}
			e.EnsureTargetHints()
			return e.Pending().Options[o.Index]
		}
	}
	t.Fatalf("no cast option for %d: %+v", id, d.Options)
	return decision.Option{}
}

// targetSplit submits the option on a clone and counts the target decision's
// options by controlling seat.
func targetSplit(t *testing.T, e *Engine, opt decision.Option) (own, opp int) {
	t.Helper()
	c := e.Clone()
	d := c.Pending()
	if err := c.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{opt.Index}}); err != nil {
		t.Fatal(err)
	}
	td := c.Pending()
	if td == nil || td.Kind != decision.KTarget {
		t.Fatalf("pending after choosing %q = %+v, want KTarget", opt.Label, td)
	}
	for _, o := range td.Options {
		if o.Player == d.Player {
			own++
		} else {
			opp++
		}
	}
	return own, opp
}

func TestTargetHintsStoneRain(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		ownN, oppN   int
		wantOwn, opp int
	}{
		{"own only", 2, 0, 2, 0},
		{"opposing only", 0, 3, 0, 3},
		{"both", 1, 2, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, _, id := newFixtureDeck(t, 31, hintStoneRain)
			for i := 0; i < tc.ownN; i++ {
				putToken(t, e, 0, hintLandSrc, state.ZBattlefield)
			}
			for i := 0; i < tc.oppN; i++ {
				putToken(t, e, 1, hintLandSrc, state.ZBattlefield)
			}
			addMana(t, e, 0, "RRR")
			opt := hintedCast(t, e, id)
			h := opt.TargetHints
			if !h.Present() || !h.Hostile() || h.Own() != tc.wantOwn || h.Opp() != tc.opp {
				t.Fatalf("hint present=%v hostile=%v own=%d opp=%d, want own=%d opp=%d",
					h.Present(), h.Hostile(), h.Own(), h.Opp(), tc.wantOwn, tc.opp)
			}
			own, opp := targetSplit(t, e, opt)
			if own != h.Own() || opp != h.Opp() {
				t.Fatalf("target decision split own=%d opp=%d, hint own=%d opp=%d", own, opp, h.Own(), h.Opp())
			}
			// Idempotent, and the engine filtered nothing: the option is offered.
			e.EnsureTargetHints()
		})
	}
}

func TestTargetHintsAnyTargetCountsPlayers(t *testing.T) {
	t.Parallel()
	e, _, id := newFixtureDeck(t, 32, hintBolt)
	addMana(t, e, 0, "R")
	opt := hintedCast(t, e, id)
	h := opt.TargetHints
	if !h.Hostile() || h.Own() != 1 || h.Opp() != 1 {
		t.Fatalf("Any-target bolt: own=%d opp=%d, want 1 and 1 (the two players)", h.Own(), h.Opp())
	}
	own, opp := targetSplit(t, e, opt)
	if own != h.Own() || opp != h.Opp() {
		t.Fatalf("split own=%d opp=%d vs hint %d/%d", own, opp, h.Own(), h.Opp())
	}
}

func TestTargetHintsAbsentForNonHostileAndUntargeted(t *testing.T) {
	t.Parallel()
	e, _, id := newFixtureDeck(t, 33, hintGrowth)
	putToken(t, e, 0, "Name:Hint Bear\nTypes:Creature Bear\nPT:2/2\nOracle:x\n", state.ZBattlefield)
	addMana(t, e, 0, "G")
	if o := hintedCast(t, e, id); o.TargetHints != 0 {
		t.Fatalf("a pump spell carries a hint: %#x", uint32(o.TargetHints))
	}
	for _, o := range e.Pending().Options {
		if o.Kind != "cast" && o.TargetHints != 0 {
			t.Fatalf("non-cast option %q carries a hint", o.Label)
		}
	}
}

func TestTargetHintsAbilityOption(t *testing.T) {
	t.Parallel()
	src := "Name:Pinger Hint\nManaCost:1\nTypes:Artifact\nA:AB$ DealDamage | Cost$ T | ValidTgts$ Creature | NumDmg$ 1\nOracle:x\n"
	e, _, _ := newFixtureDeck(t, 34, "Name:Idle Hint\nManaCost:W\nTypes:Sorcery\nA:SP$ Draw\nOracle:x\n")
	pinger := putToken(t, e, 0, src, state.ZBattlefield)
	putToken(t, e, 0, "Name:Hint Bear\nTypes:Creature Bear\nPT:2/2\nOracle:x\n", state.ZBattlefield)
	addMana(t, e, 0, "R")
	e.EnsureTargetHints()
	var found bool
	for _, o := range e.Pending().Options {
		if o.Kind == "ability" && o.Obj == pinger {
			found = true
			h := o.TargetHints
			if !h.Hostile() || h.Own() != 1 || h.Opp() != 0 {
				t.Fatalf("pinger hint own=%d opp=%d hostile=%v, want 1/0", h.Own(), h.Opp(), h.Hostile())
			}
			own, opp := targetSplit(t, e, o)
			if own != 1 || opp != 0 {
				t.Fatalf("target decision split %d/%d", own, opp)
			}
		}
	}
	if !found {
		t.Fatal("ability option not offered")
	}
}
