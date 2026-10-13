package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// Sacred Boon / Scars of the Veteran: the DB$ Effect prevention rider. Each
// shield application registers one end-step delayed +0/+1 counter body that
// reads the amount THAT application prevented.

func boonSetup(t *testing.T, name string, target func(lion state.ObjID) state.Target) (*Engine, state.ObjID, state.ObjID) {
	t.Helper()
	reg := sharedCorpus(t)
	e := newSeats(t, 2)
	e.pending = nil
	card := onBoardCard(t, e, 0, mustCorpusCard(t, reg, name))
	lion := onBoard(t, e, 0, "Name:Lion\nTypes:Creature\nPT:2/2\nOracle:x\n")
	effects.Resolve(e, &effects.Ctx{Source: card, Controller: 0,
		Targets:        []state.Target{target(lion)},
		TargetsOffered: true}, e.G.Obj(card).Face().SpellAbility())
	if got := countShieldCEs(e); got != 1 {
		t.Fatalf("shields = %d, want 1", got)
	}
	if e.G.Obj(lion).Zone != state.ZBattlefield {
		t.Fatal("precondition: lion on the battlefield")
	}
	return e, card, lion
}

func boonDamage(t *testing.T, e *Engine, to state.ObjID, n int32) {
	t.Helper()
	source := onBoard(t, e, 1, "Name:Bolter\nManaCost:R\nTypes:Creature\nPT:1/1\nOracle:x\n")
	e.damaging = source
	e.emit(events.Event{Kind: events.Damage, Obj: to, Amount: n})
	e.damaging = 0
}

func fireEndStep(e *Engine) {
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepEnd})
	for i := 0; i < 8; i++ {
		e.putTriggersOnStack()
		if d := e.Pending(); d != nil && d.Kind == decision.KTriggerOrder {
			choices := make([]int, len(d.Options))
			for j := range choices {
				choices[j] = j
			}
			if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: choices}); err != nil {
				panic(err)
			}
		}
		if len(e.G.Stack) == 0 {
			return
		}
		e.resolveTop()
	}
}

func TestSacredBoonRiderCountersTotalPrevented(t *testing.T) {
	t.Parallel()
	e, _, lion := boonSetup(t, "Sacred Boon", func(l state.ObjID) state.Target { return state.Target{Obj: l} })
	boonDamage(t, e, lion, 2)
	boonDamage(t, e, lion, 1)
	if len(e.G.Delayed) != 2 {
		t.Fatalf("registrations = %d, want one per application (2)", len(e.G.Delayed))
	}
	if got := e.G.Obj(lion).Counter("P0P1"); got != 0 {
		t.Fatalf("counters before the end step = %d, want 0", got)
	}
	fireEndStep(e)
	if got := e.G.Obj(lion).Counter("P0P1"); got != 3 {
		t.Fatalf("P0P1 counters = %d, want 3", got)
	}
	assertNoUnimplementedNotes(t, e)
}

func TestSacredBoonRiderCapsAtAmount(t *testing.T) {
	t.Parallel()
	e, _, lion := boonSetup(t, "Sacred Boon", func(l state.ObjID) state.Target { return state.Target{Obj: l} })
	boonDamage(t, e, lion, 5)
	fireEndStep(e)
	if got := e.G.Obj(lion).Counter("P0P1"); got != 3 {
		t.Fatalf("P0P1 counters = %d, want 3 (5 damage vs Amount 3)", got)
	}
	assertNoUnimplementedNotes(t, e)
}

func TestSacredBoonRiderCreatureGoneGetsNothing(t *testing.T) {
	t.Parallel()
	e, _, lion := boonSetup(t, "Sacred Boon", func(l state.ObjID) state.Target { return state.Target{Obj: l} })
	boonDamage(t, e, lion, 2)
	if len(e.G.Delayed) != 1 {
		t.Fatalf("precondition: registrations = %d, want 1", len(e.G.Delayed))
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: lion, From: state.ZBattlefield, To: state.ZGraveyard})
	if e.G.Obj(lion).Zone == state.ZBattlefield {
		t.Fatal("precondition: lion left the battlefield")
	}
	fireEndStep(e)
	if got := e.G.Obj(lion).Counter("P0P1"); got != 0 {
		t.Fatalf("P0P1 counters = %d, want 0", got)
	}
	assertNoUnimplementedNotes(t, e)
}

func TestSacredBoonRiderZeroPreventedRegistersNothing(t *testing.T) {
	t.Parallel()
	e, _, lion := boonSetup(t, "Sacred Boon", func(l state.ObjID) state.Target { return state.Target{Obj: l} })
	// Damage to a different creature is not prevented by this shield.
	other := onBoard(t, e, 0, "Name:Bear\nTypes:Creature\nPT:2/2\nOracle:x\n")
	boonDamage(t, e, other, 2)
	if len(e.G.Delayed) != 0 {
		t.Fatalf("registrations = %d, want 0", len(e.G.Delayed))
	}
	if countShieldCEs(e) != 1 || e.G.Obj(lion).Counter("P0P1") != 0 {
		t.Fatal("precondition: shield still up on the lion")
	}
	assertNoUnimplementedNotes(t, e)
}

func TestScarsOfTheVeteranRider(t *testing.T) {
	t.Parallel()
	t.Run("creature", func(t *testing.T) {
		e, _, lion := boonSetup(t, "Scars of the Veteran", func(l state.ObjID) state.Target { return state.Target{Obj: l} })
		boonDamage(t, e, lion, 2)
		fireEndStep(e)
		if got := e.G.Obj(lion).Counter("P0P1"); got != 2 {
			t.Fatalf("P0P1 counters = %d, want 2", got)
		}
		assertNoUnimplementedNotes(t, e)
	})
	t.Run("player", func(t *testing.T) {
		e, _, lion := boonSetup(t, "Scars of the Veteran", func(state.ObjID) state.Target {
			return state.Target{Player: 0, IsPlayer: true}
		})
		e.damaging = lion
		e.emit(events.Event{Kind: events.Damage, Player: 0, Amount: 2})
		e.damaging = 0
		if got := e.G.Players[0].Life; got != 20 {
			t.Fatalf("life = %d, want 20 (damage prevented)", got)
		}
		fireEndStep(e)
		if got := e.G.Obj(lion).Counter("P0P1"); got != 0 {
			t.Fatalf("lion counters = %d, want 0", got)
		}
		assertNoUnimplementedNotes(t, e)
	})
}
