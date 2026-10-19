package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// Soulstone Sanctuary's "{4}: This land becomes a 3/3 creature with
// vigilance and all creature types" names the permanent that activated it.
// When the Sanctuary leaves the battlefield with the ability on the stack
// (Uncharted Voyage put it into its library), the card is a new object the
// resolving ability does not reach (CR 400.7): drawn later it is a plain
// land, not a 3/3 with vigilance in hand (census pair 3988). An animation
// that did resolve also ends with the permanent (CR 611.2c).
func TestAnimationDoesNotReachASourceThatLeft(t *testing.T) {
	for _, leaveFirst := range []bool{false, true} {
		e, _ := fdnCensusEngine(t, "Soulstone Sanctuary")
		soul := moveByName(t, e, 0, "Soulstone Sanctuary", state.ZBattlefield)
		addMana(t, e, 0, "CCCC")
		d := e.Pending()
		idx := -1
		for _, o := range d.Options {
			if o.Kind == "ability" && o.Obj == soul {
				idx = o.Index
			}
		}
		if idx < 0 {
			t.Fatalf("no animate option: %+v", d.Options)
		}
		submitChoices(t, e, idx)
		if leaveFirst {
			e.emit(events.Event{Kind: events.MoveZone, Obj: soul, From: state.ZBattlefield, To: state.ZLibrary})
		}
		fdnCensusDrain(t, e)
		if !leaveFirst {
			if !e.IsCreature(soul) {
				t.Fatalf("precondition: Soulstone not a creature after animating: %v", e.Derived(soul).Types)
			}
			e.emit(events.Event{Kind: events.MoveZone, Obj: soul, From: state.ZBattlefield, To: state.ZLibrary})
		}
		// Drawn on a later turn, as in the census game.
		e.emit(events.Event{Kind: events.TurnChange, Player: 1, Amount: e.G.Turn + 1})
		e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: e.G.Turn + 1})
		e.emit(events.Event{Kind: events.Draw, Player: 0, Obj: soul, From: state.ZLibrary, To: state.ZHand})
		// The seat's view of the card in hand (what policynet featurizes).
		if _, _, kw, p, tt := e.ViewCharacteristics(soul); e.IsCreature(soul) || p != 0 || tt != 0 || len(kw) != 0 {
			t.Errorf("leaveFirst=%v: the Sanctuary in hand shows %d/%d %v, want the printed land", leaveFirst, p, tt, kw)
		}
		e.emit(events.Event{Kind: events.MoveZone, Obj: soul, From: state.ZHand, To: state.ZBattlefield})
		if e.IsCreature(soul) {
			t.Errorf("leaveFirst=%v: the replayed Sanctuary is animated: %v", leaveFirst, e.Derived(soul).Types)
		}
	}
}
