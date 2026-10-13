package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// TestShuriOrnithopterBecomesSolRing: Shuri's Pump root targets "artifact you
// control" and its DBClone sub (CloneTarget$ ParentTarget, a SECOND target)
// names the root's target as the object that becomes the copy. The sub's own
// target is the copy source (Sol Ring).
func TestShuriOrnithopterBecomesSolRing(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e, _ := corpusEngineCfg(t, reg,
		[]*cards.Card{lookup(t, reg, "Shuri, Wakandan Inventor"), lookup(t, reg, "Ornithopter"), lookup(t, reg, "Sol Ring")},
		nil)
	shuri := moveByName(t, e, 0, "Shuri, Wakandan Inventor", state.ZBattlefield)
	orni := moveByName(t, e, 0, "Ornithopter", state.ZBattlefield)
	ring := moveByName(t, e, 0, "Sol Ring", state.ZBattlefield)
	// Test-only setup write; it is not an event, so no replayCheck here.
	e.G.Obj(shuri).SummonSick = false
	e.pending = nil
	addMana(t, e, 0, "U")
	e.Advance()
	if o := e.G.Obj(orni); o.Zone != state.ZBattlefield || o.Face().Name != "Ornithopter" {
		t.Fatalf("precondition: Ornithopter zone=%v name=%q", o.Zone, o.Face().Name)
	}
	submitChoices(t, e, abilityOption(t, e, shuri, 0).Index)
	for i := 0; i < 40; i++ {
		d := e.Pending()
		if d == nil {
			t.Fatal("no pending decision")
		}
		if d.Kind == decision.KPriority {
			if len(e.G.Stack) == 0 {
				break
			}
			passPriority(t, e)
			continue
		}
		pick := -1
		for _, o := range d.Options {
			if o.Obj == orni {
				pick = o.Index
			}
		}
		if pick < 0 {
			for _, o := range d.Options {
				if o.Obj == ring {
					pick = o.Index
				}
			}
		}
		if pick < 0 {
			t.Fatalf("decision %v offers neither artifact: %+v", d.Kind, d.Options)
		}
		submitChoices(t, e, pick)
	}
	if f := e.G.Obj(orni).Face(); f == nil || f.Name != "Sol Ring" {
		t.Fatalf("Ornithopter face = %v, want Sol Ring", f)
	}
}
