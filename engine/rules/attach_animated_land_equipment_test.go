package rules

import (
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"testing"
)

// CR 301.5c / 704.5n: an Equipment stays attached to a permanent that is a
// creature. An animated Soulstone Sanctuary ({4}: it becomes a 3/3 creature,
// still a land) is a creature by its DERIVED types; the SBA used to read the
// printed face (Land) and unattach the Equipment at the next check.
func TestEquipmentStaysOnAnimatedSoulstone(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Soulstone Sanctuary", "Bonesplitter")
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
	fdnCensusDrain(t, e)
	if !e.IsCreature(soul) {
		t.Fatalf("Soulstone not a creature after animating: %v", e.Derived(soul).Types)
	}
	bs := moveByName(t, e, 0, "Bonesplitter", state.ZBattlefield)
	e.emit(events.Event{Kind: events.Attach, Obj: bs, IDs: []state.ObjID{soul}})
	e.checkStateBased()
	if got := e.G.Obj(bs).AttachedTo; got != soul {
		t.Fatalf("Bonesplitter attached to %d after SBA, want the animated Soulstone %d", got, soul)
	}
	// Across a turn boundary too (the animation is permanent).
	submitPass(t, e)
	for i := 0; i < 60 && e.G.Turn < 3; i++ {
		d := e.Pending()
		if d == nil {
			break
		}
		if d.Kind == decision.KPriority {
			submitPass(t, e)
		} else {
			fdnCensusAnswerMin(t, e, d)
		}
	}
	if got := e.G.Obj(bs).AttachedTo; got != soul || !e.IsCreature(soul) {
		t.Fatalf("turn %d: Bonesplitter attached to %d (want %d), Soulstone creature=%v types %v", e.G.Turn, got, soul, e.IsCreature(soul), e.Derived(soul).Types)
	}
}
