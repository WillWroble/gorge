package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

// TestPucasCovenantTargetsAnotherCard pins Puca's Covenant's
// `ValidTgts$ Permanent.YouOwn+cmcLEX+NotDefinedTriggeredCard` ("return
// ANOTHER target permanent card ... from your graveyard"): when a creature
// with two counters dies, the push-time target ask offers a qualifying card
// already in the graveyard and never the dead creature itself, though that
// card satisfies every other clause (a permanent card you own, mana value 2
// <= 2 counters). Before NotDefinedTriggeredCard was recognised the whole
// spec failed closed and the ask offered nothing.
func TestPucasCovenantTargetsAnotherCard(t *testing.T) {
	t.Parallel()
	e, _, mine, _ := cr601Board(t, 61031,
		map[string]state.Zone{"Puca's Covenant": state.ZBattlefield, "Grizzly Bears": state.ZBattlefield, "Llanowar Elves": state.ZGraveyard},
		map[string]state.Zone{})
	bears, elves := mine["Grizzly Bears"], mine["Llanowar Elves"]
	putCountersOn(t, e, 0, "Grizzly Bears", "P1P1", 2)
	if o := e.G.Obj(bears); o == nil || o.Zone != state.ZBattlefield || o.Counter("P1P1") != 2 || o.Face().ManaValue() != 2 {
		t.Fatalf("precondition: Grizzly Bears must be a mana value 2 battlefield creature with two counters: %+v", o)
	}
	if o := e.G.Obj(elves); o == nil || o.Zone != state.ZGraveyard || o.Owner != 0 {
		t.Fatalf("precondition: Llanowar Elves must be in seat 0's graveyard: %+v", o)
	}

	killOnBattlefield(t, e, bears)
	if o := e.G.Obj(bears); o == nil || o.Zone != state.ZGraveyard {
		t.Fatalf("precondition: the dead Grizzly Bears must be in the graveyard (where the target spec looks): %+v", o)
	}
	var ask *decision.Decision
	for i := 0; i < 20; i++ {
		d := e.Pending()
		if d == nil {
			break
		}
		if d.Kind == decision.KTarget {
			ask = d
			break
		}
		if d.Kind != decision.KPriority || len(e.G.Stack) == 0 {
			t.Fatalf("unexpected decision before Puca's Covenant's target ask: %+v", d)
		}
		submitChoices(t, e, passIndex(t, d))
	}
	if ask == nil {
		t.Fatalf("Puca's Covenant's death trigger posed no target ask (stack %v)", e.G.Stack)
	}
	offered := map[state.ObjID]bool{}
	for _, o := range ask.Options {
		offered[o.Obj] = true
	}
	if !offered[elves] {
		t.Errorf("target ask %+v omits Llanowar Elves %d, another qualifying permanent card", ask.Options, elves)
	}
	if offered[bears] {
		t.Errorf("target ask %+v offers the dead Grizzly Bears %d itself; Oracle says \"another\"", ask.Options, bears)
	}
}
