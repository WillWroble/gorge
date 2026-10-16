package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// relatedE2EDrive answers every non-priority ask of a real exchange trigger
// until it resolves: a KTarget ask picks the option for pick[n] (n-th target
// ask), anything else (the OptionalDecider$ may) answers index 0 / yes. It
// returns the option objects offered at each target ask.
func relatedE2EDrive(t *testing.T, e *Engine, pick []state.ObjID) [][]state.ObjID {
	t.Helper()
	var offered [][]state.ObjID
	for n := 0; n < 12; n++ {
		d := passUntilAsk(t, e)
		if d == nil {
			break
		}
		if d.Kind != decision.KTarget {
			t.Logf("ask %v %+v", d.Kind, d.Options)
			submitChoices(t, e, 0)
			continue
		}
		var ids []state.ObjID
		idx := -1
		for _, o := range d.Options {
			ids = append(ids, o.Obj)
			if len(offered) < len(pick) && o.Obj == pick[len(offered)] {
				idx = o.Index
			}
		}
		if len(offered) >= len(pick) || idx < 0 {
			t.Fatalf("target ask %d (%s) offered %v, wanted %v", len(offered), d.ResumeKind, ids, pick)
		}
		offered = append(offered, ids)
		submitChoices(t, e, idx)
		if len(offered) == len(pick) {
			break
		}
	}
	return offered
}

// relatedE2EResolve passes priority and answers the OptionalDecider$ "yes"
// until the stack empties (the exchange trigger has resolved).
func relatedE2EResolve(t *testing.T, e *Engine) {
	t.Helper()
	for i := 0; i < 200; i++ {
		d := e.Pending()
		if d == nil {
			e.Advance()
			continue
		}
		if d.Kind != decision.KPriority {
			if d.Kind != decision.KTriggerOptional {
				t.Fatalf("unexpected ask %+v", d)
			}
			submitChoices(t, e, 0)
			continue
		}
		if len(e.G.Stack) == 0 {
			return
		}
		for _, o := range d.Options {
			if o.Kind == "pass" {
				submitChoices(t, e, o.Index)
				break
			}
		}
	}
	t.Fatal("trigger never resolved")
}

func relatedHas(ids []state.ObjID, id state.ObjID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestSpawnbrokerExchangeEndToEnd(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	e := chainAskDeck(t, reg, "Spawnbroker", "Grizzly Bears")
	bears := searchMoveByName(t, e, "Grizzly Bears", state.ZBattlefield)
	small := putBattlefield(t, e, 1, "Name:Small\nManaCost:1\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")
	big := putBattlefield(t, e, 1, "Name:Big\nManaCost:5\nTypes:Creature Bear\nPT:5/5\nOracle:x\n")
	if e.Power(big) <= e.Power(bears) || e.Power(small) != e.Power(bears) || e.Power(bears) == 0 {
		t.Fatalf("precondition: powers bears=%d small=%d big=%d", e.Power(bears), e.Power(small), e.Power(big))
	}
	brk := searchMoveByName(t, e, "Spawnbroker", state.ZBattlefield)
	if e.G.Obj(brk).Zone != state.ZBattlefield {
		t.Fatal("Spawnbroker not on the battlefield")
	}
	offered := relatedE2EDrive(t, e, []state.ObjID{bears, small})
	if len(offered) != 2 {
		t.Fatalf("target asks = %d, want 2", len(offered))
	}
	if !relatedHas(offered[1], small) || relatedHas(offered[1], big) {
		t.Fatalf("second ask offered %v: want small %d and not big %d", offered[1], small, big)
	}
	relatedE2EResolve(t, e)
	if got := e.G.Obj(small).Controller; got != 0 {
		t.Fatalf("Small controller = %d, want 0 after the exchange", got)
	}
	if got := e.G.Obj(bears).Controller; got != 1 {
		t.Fatalf("Bears controller = %d, want 1 after the exchange", got)
	}
}

func TestPucasMischiefExchangeEndToEnd(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	e := chainAskDeck(t, reg, "Puca's Mischief", "Grizzly Bears")
	bears := searchMoveByName(t, e, "Grizzly Bears", state.ZBattlefield)
	puca := searchMoveByName(t, e, "Puca's Mischief", state.ZBattlefield)
	cheap := putBattlefield(t, e, 1, "Name:Cheap\nManaCost:1\nTypes:Artifact\nOracle:x\n")
	dear := putBattlefield(t, e, 1, "Name:Dear\nManaCost:5\nTypes:Artifact\nOracle:x\n")
	cmc := func(id state.ObjID) int32 { return e.G.Obj(id).Face().Cmc() }
	if cmc(bears) != 2 || cmc(cheap) != 1 || cmc(dear) != 5 || e.G.Obj(puca).Zone != state.ZBattlefield {
		t.Fatalf("precondition: cmc bears=%d cheap=%d dear=%d", cmc(bears), cmc(cheap), cmc(dear))
	}
	offered := relatedE2EDrive(t, e, []state.ObjID{bears, cheap})
	if len(offered) != 2 {
		t.Fatalf("target asks = %d, want 2", len(offered))
	}
	if !relatedHas(offered[1], cheap) || relatedHas(offered[1], dear) {
		t.Fatalf("second ask offered %v: want cheap %d and not dear %d", offered[1], cheap, dear)
	}
	relatedE2EResolve(t, e)
	if got := e.G.Obj(cheap).Controller; got != 0 {
		t.Fatalf("Cheap controller = %d, want 0 after the exchange", got)
	}
	if got := e.G.Obj(bears).Controller; got != 1 {
		t.Fatalf("Bears controller = %d, want 1 after the exchange", got)
	}
}
