package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// Hungry Ghoul's "{1}, Sacrifice another creature: Put a +1/+1 counter on
// this creature" acts on the permanent that activated it. If the Ghoul dies
// with the ability on the stack, the card in the graveyard is a new object
// (CR 400.7) and the counter is not put anywhere; it used to land on the
// graveyard card (census pair 555). A Ghoul that stays gets its counter.
func TestPutCounterOnSourceThatLeftDoesNothing(t *testing.T) {
	for _, dies := range []bool{false, true} {
		e, _ := fdnCensusEngine(t, "Hungry Ghoul", "Grizzly Bears")
		ghoul := moveByName(t, e, 0, "Hungry Ghoul", state.ZBattlefield)
		moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
		addMana(t, e, 0, "C")
		d := e.Pending()
		idx := -1
		for _, o := range d.Options {
			if o.Kind == "ability" && o.Obj == ghoul {
				idx = o.Index
			}
		}
		if idx < 0 {
			t.Fatalf("no Hungry Ghoul activation: %+v", d.Options)
		}
		submitChoices(t, e, idx)
		for i := 0; i < 5 && len(e.G.Stack) == 0; i++ {
			// Answer the sacrifice choice, if posed.
			if d := e.Pending(); d != nil && d.Kind != "priority" {
				submitChoices(t, e, 0)
			} else {
				break
			}
		}
		if len(e.G.Stack) != 1 {
			t.Fatalf("dies=%v: stack %v after activating, want the ability", dies, e.G.Stack)
		}
		if dies {
			e.emit(events.Event{Kind: events.MoveZone, Obj: ghoul, From: state.ZBattlefield, To: state.ZGraveyard})
		}
		fdnCensusDrain(t, e)
		g := e.G.Obj(ghoul)
		want := 1
		if dies {
			want = 0
		}
		if got := g.Counter("P1P1"); int(got) != want {
			t.Fatalf("dies=%v: Hungry Ghoul in %s has %d +1/+1 counters, want %d", dies, g.Zone, got, want)
		}
	}
}

// Quilled Greatwurm's "put that many +1/+1 counters on it (It must survive
// to get the counters.)" names the damage source as it was (Defined$
// TriggeredSourceLKICopy). A creature that died to the same combat damage is
// a new object in the graveyard and gets nothing (census pair 7476); a
// survivor gets the counters.
func TestPutCounterOnTriggeredLKICopyThatLeftDoesNothing(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Quilled Greatwurm", "Grizzly Bears", "Grizzly Bears")
	wurm := moveByName(t, e, 0, "Quilled Greatwurm", state.ZBattlefield)
	alive := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	dead := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	e.emit(events.Event{Kind: events.MoveZone, Obj: dead, From: state.ZBattlefield, To: state.ZGraveyard})
	sa := resolveSourceFaceSA(t, e, wurm, "TrigPutCounter")
	for _, src := range []state.ObjID{alive, dead} {
		e.pending = nil
		e.probe(func() {
			effects.Resolve(e, &effects.Ctx{Source: wurm, Controller: 0, SVars: e.G.Obj(wurm).Face().SVars,
				TriggerContext: effects.TriggerContext{TriggerSource: src, TriggerAmount: 2}}, sa)
		})
	}
	if got := e.G.Obj(alive).Counter("P1P1"); got != 2 {
		t.Fatalf("the surviving Bears has %d +1/+1 counters, want 2", got)
	}
	if got := e.G.Obj(dead).Counter("P1P1"); got != 0 {
		t.Fatalf("the Bears in the graveyard has %d +1/+1 counters, want 0", got)
	}
}
