package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// A DamageDoneOnce | ValidSource$ Card.Self | DamageAmount$ GE4 line queues one
// entry per damaged referent. closeDamageBatch must drop only the entry whose
// total fails the gate: a failing entry (2 to player 1) queued BEFORE a passing
// one (5 to player 0) must not kill the later entry's adjacent slot, whichever
// order the hits arrive in.
func TestDamageAmountGatePerReferentEntriesAreIndependent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		first int32
		next  int32
	}{{"fail-then-pass", 2, 5}, {"pass-then-fail", 5, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			reg := testutil.CorpusRegistry(t)
			src := "Name:Dealer\nManaCost:1 R\nTypes:Creature\nPT:2/9\n" +
				"T:Mode$ DamageDoneOnce | ValidSource$ Card.Self | DamageAmount$ GE4 | TriggerZones$ Battlefield | Execute$ TrigPump\n" +
				"SVar:TrigPump:DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | CounterNum$ 1\nOracle:x\n"
			e, _ := combatTriggerBoard(t, reg, nil, []string{src}, nil, nil)
			e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: 2})
			e.emit(events.Event{Kind: events.StepChange, Step: state.StepMain1})
			id := findBattlefield(t, e, 0, "Dealer", 0)
			if o := e.G.Obj(id); o == nil || o.Zone != state.ZBattlefield {
				t.Fatalf("carrier = %+v, want battlefield object", o)
			}
			e.damaging = id
			e.combatDamaging = true
			e.BeginDamageBatch()
			// Two distinct referents: player 1 then player 0.
			e.emit(events.Event{Kind: events.Damage, Player: 1, Amount: tc.first})
			e.emit(events.Event{Kind: events.Damage, Player: 0, Amount: tc.next})
			if len(e.pendingTriggers) != 2 {
				t.Fatalf("precondition: %d entries queued mid-batch, want 2 per-referent", len(e.pendingTriggers))
			}
			e.EndDamageBatch()
			e.combatDamaging = false
			e.damaging = 0
			if n := len(e.pendingTriggers); n != 1 {
				t.Fatalf("pendingTriggers after close = %d, want 1 (only the 5-damage referent passes GE4)", n)
			}
		})
	}
}
