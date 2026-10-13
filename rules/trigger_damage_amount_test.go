package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// DamageAmount$ on a trigger line compares the damage amount (the batch TOTAL
// on the *Once modes). Innocent Bystander: Shock (2) must not trigger, Bolt (3)
// must, and two simultaneous 2-damage hits are "dealt 3 or more".
func damageAmountBoard(t *testing.T, mode, params string) (*Engine, state.ObjID) {
	t.Helper()
	reg := testutil.CorpusRegistry(t)
	src := "Name:Amount Gate\nManaCost:1 G\nTypes:Creature\nPT:2/9\n" +
		"T:Mode$ " + mode + " | ValidTarget$ Card.Self | " + params + " | TriggerZones$ Battlefield | Execute$ TrigPump\n" +
		"SVar:TrigPump:DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 1\n" +
		"SVar:Three:3\nOracle:x\n"
	e, _ := combatTriggerBoard(t, reg, nil, []string{src}, nil, nil)
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: 2})
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepMain1})
	id := findBattlefield(t, e, 0, "Amount Gate", 0)
	if o := e.G.Obj(id); o == nil || o.Zone != state.ZBattlefield {
		t.Fatalf("carrier = %+v, want battlefield object", o)
	}
	// Precondition: the gate line must be registered (not "unimplemented").
	if _, ok := e.G.Obj(id).Face().Triggers[0].Param(cards.PKDamageAmount); !ok {
		t.Fatal("carrier trigger has no DamageAmount$")
	}
	return e, id
}

func damageAmountFires(t *testing.T, e *Engine, id state.ObjID, amounts ...int32) bool {
	t.Helper()
	e.BeginDamageBatch()
	for _, a := range amounts {
		e.emit(events.Event{Kind: events.Damage, Obj: id, Amount: a})
	}
	e.EndDamageBatch()
	n := len(e.pendingTriggers)
	e.putTriggersOnStack()
	for len(e.G.Stack) > 0 {
		e.resolveTop()
	}
	return n > 0
}

func TestDamageDoneOnceDamageAmountGE3(t *testing.T) {
	t.Parallel()
	e, id := damageAmountBoard(t, "DamageDoneOnce", "DamageAmount$ GE3")
	if damageAmountFires(t, e, id, 2) {
		t.Fatal("Shock (2) triggered GE3")
	}
	if !damageAmountFires(t, e, id, 3) {
		t.Fatal("Bolt (3) did not trigger GE3")
	}
	if got := e.G.Obj(id).Counter("P1P1"); got != 1 {
		t.Fatalf("counters = %d, want 1", got)
	}
}

func TestDamageDoneOnceDamageAmountUsesBatchTotal(t *testing.T) {
	t.Parallel()
	e, id := damageAmountBoard(t, "DamageDoneOnce", "DamageAmount$ GE3")
	if !damageAmountFires(t, e, id, 2, 2) {
		t.Fatal("2+2 in one batch (total 4) did not trigger GE3")
	}
	if damageAmountFires(t, e, id, 1, 1) {
		t.Fatal("1+1 in one batch (total 2) triggered GE3")
	}
	if got := e.G.Obj(id).Counter("P1P1"); got != 1 {
		t.Fatalf("counters = %d, want exactly 1", got)
	}
}

func TestDamageDoneDamageAmountPerEventAndSVarOperand(t *testing.T) {
	t.Parallel()
	e, id := damageAmountBoard(t, "DamageDone", "DamageAmount$ EQ1")
	if damageAmountFires(t, e, id, 2) {
		t.Fatal("2 damage triggered EQ1")
	}
	if !damageAmountFires(t, e, id, 1) {
		t.Fatal("1 damage did not trigger EQ1")
	}
	e2, id2 := damageAmountBoard(t, "DamageDone", "DamageAmount$ GEThree")
	if damageAmountFires(t, e2, id2, 2) {
		t.Fatal("2 damage triggered GEThree (SVar 3)")
	}
	if !damageAmountFires(t, e2, id2, 3) {
		t.Fatal("3 damage did not trigger GEThree (SVar 3)")
	}
}
