package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// Goldvein Pick's equip ability attaches the Pick that activated it. If the
// Pick is destroyed with the ability on the stack, the card in the graveyard
// is a new object (CR 400.7) and nothing is attached (CR 701.3b); it used to
// be attached from the graveyard (census pair 7781). A Pick flickered with
// the ability on the stack is a new object on the battlefield and stays
// unattached too. An untouched Pick is attached.
func TestEquipOnAnEquipmentThatLeftDoesNothing(t *testing.T) {
	for _, mode := range []string{"stays", "dies", "flicker"} {
		e, _ := fdnCensusEngine(t, "Goldvein Pick", "Grizzly Bears")
		pick := moveByName(t, e, 0, "Goldvein Pick", state.ZBattlefield)
		bears := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
		addMana(t, e, 0, "C")
		d := e.Pending()
		idx := -1
		for _, o := range d.Options {
			if o.Kind == "ability" && o.Obj == pick {
				idx = o.Index
			}
		}
		if idx < 0 {
			t.Fatalf("%s: no Goldvein Pick equip activation: %+v", mode, d.Options)
		}
		submitChoices(t, e, idx)
		for i := 0; i < 5 && len(e.G.Stack) == 0; i++ {
			d := e.Pending()
			if d == nil || d.Kind == decision.KPriority {
				break
			}
			submitChoices(t, e, 0)
		}
		if len(e.G.Stack) != 1 {
			t.Fatalf("%s: stack %v after equipping, want the equip ability", mode, e.G.Stack)
		}
		switch mode {
		case "dies":
			e.emit(events.Event{Kind: events.MoveZone, Obj: pick, From: state.ZBattlefield, To: state.ZGraveyard})
		case "flicker":
			e.emit(events.Event{Kind: events.MoveZone, Obj: pick, From: state.ZBattlefield, To: state.ZExile})
			e.emit(events.Event{Kind: events.MoveZone, Obj: pick, From: state.ZExile, To: state.ZBattlefield})
		}
		fdnCensusDrain(t, e)
		p := e.G.Obj(pick)
		want := state.ObjID(0)
		if mode == "stays" {
			want = bears
		}
		if p.AttachedTo != want {
			t.Fatalf("%s: Goldvein Pick in %s is attached to %d, want %d", mode, p.Zone, p.AttachedTo, want)
		}
	}
}
