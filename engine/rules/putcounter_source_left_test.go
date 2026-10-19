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

// Youthful Valkyrie's "whenever another Angel you control enters, put a
// +1/+1 counter on this creature" acts on the permanent that triggered. If
// the Valkyrie leaves the battlefield after the Angel enters but before the
// trigger is put on the stack (Abyssal Harvester's copy enters, then the same
// resolution exiles the other Nightmare tokens), the trigger has no
// battlefield push stamp; it must still do nothing to the new object in the
// graveyard (CR 400.7; census pair 3285). A Valkyrie that stays gets its
// counter.
func TestPutCounterOnSourceThatLeftBeforeItsTriggerWasPushedDoesNothing(t *testing.T) {
	for _, leaves := range []bool{false, true} {
		e, _ := fdnCensusEngine(t, "Youthful Valkyrie", "Vanguard Seraph")
		valk := moveByName(t, e, 0, "Youthful Valkyrie", state.ZBattlefield)
		e.pending = nil
		moveByName(t, e, 0, "Vanguard Seraph", state.ZBattlefield)
		if len(e.pendingTriggers) == 0 {
			t.Fatal("precondition: the Angel's entry queued no Valkyrie trigger")
		}
		if leaves {
			e.emit(events.Event{Kind: events.MoveZone, Obj: valk, From: state.ZBattlefield, To: state.ZGraveyard})
		}
		e.putTriggersOnStack()
		for i := 0; i < 5 && len(e.G.Stack) > 0; i++ {
			e.resolveTop()
		}
		want := int32(1)
		if leaves {
			want = 0
		}
		if got := e.G.Obj(valk).Counter("P1P1"); got != want {
			t.Fatalf("leaves=%v: Youthful Valkyrie in %s has %d +1/+1 counters, want %d", leaves, e.G.Obj(valk).Zone, got, want)
		}
	}
}

// Beast-Kin Ranger's "whenever another creature you control enters, this
// creature gets +1/+0 until end of turn" pumps the permanent that triggered.
// A Ranger flickered with the trigger on the stack is a new object (CR
// 400.7): the pump must not reach it. Untouched, it gets the pump.
func TestPumpOnAFlickeredSourceDoesNothing(t *testing.T) {
	for _, flicker := range []bool{false, true} {
		e, _ := fdnCensusEngine(t, "Beast-Kin Ranger", "Grizzly Bears")
		ranger := moveByName(t, e, 0, "Beast-Kin Ranger", state.ZBattlefield)
		e.pending = nil
		moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
		e.putTriggersOnStack()
		if len(e.G.Stack) == 0 {
			t.Fatal("precondition: no Ranger trigger on the stack")
		}
		if flicker {
			e.emit(events.Event{Kind: events.MoveZone, Obj: ranger, From: state.ZBattlefield, To: state.ZExile})
			e.emit(events.Event{Kind: events.MoveZone, Obj: ranger, From: state.ZExile, To: state.ZBattlefield})
			e.putTriggersOnStack()
		}
		for i := 0; i < 5 && len(e.G.Stack) > 0; i++ {
			e.resolveTop()
		}
		want := int32(4)
		if flicker {
			want = 3
		}
		if got := e.Power(ranger); got != want {
			t.Fatalf("flicker=%v: Beast-Kin Ranger power %d, want %d", flicker, got, want)
		}
	}
}
