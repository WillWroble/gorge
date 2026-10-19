package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// A token copy of Heraldic Banner (Extravagant Replication) copies the
// printed "As Heraldic Banner enters, choose a color" (CR 707.2), and that
// replacement applies to the copy's own entry (CR 614.12): its controller
// is asked, and the copy's chosen colour is its own answer, not the
// original's.
func TestHeraldicBannerTokenCopyAsksItsColour(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Extravagant Replication", "Heraldic Banner")
	repl := moveByName(t, e, 0, "Extravagant Replication", state.ZBattlefield)
	banner := moveByName(t, e, 0, "Heraldic Banner", state.ZBattlefield)
	e.emit(events.Event{Kind: events.Choose, Obj: banner, Player: 0, Counter: "color", Text: "W"})
	if got := e.G.Obj(banner).ChosenColor; got != "W" {
		t.Fatalf("precondition: original Banner colour %q, want W", got)
	}
	sa := resolveSourceFaceSA(t, e, repl, "TrigClone")
	e.pending = nil
	e.probe(func() {
		effects.Resolve(e, &effects.Ctx{Source: repl, Controller: 0,
			Targets: []state.Target{{Obj: banner}}}, sa)
	})
	d := e.Pending()
	if d != nil && d.Kind == decision.KChoose && len(d.Options) > 0 && d.Options[0].Obj == banner {
		submitChoices(t, e, 0)
		d = e.Pending()
	}
	if d == nil || d.Kind != decision.KChoose || d.Player != 0 {
		t.Fatalf("the Banner copy entered without its controller's colour ask: %+v", d)
	}
	idx := -1
	for _, o := range d.Options {
		if o.Label == "Red" {
			idx = o.Index
		}
	}
	if idx < 0 {
		t.Fatalf("no red option in the colour ask: %+v", d.Options)
	}
	if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{idx}}); err != nil {
		t.Fatalf("submit colour: %v", err)
	}
	cp := findTokenCopyOf(t, e, e.G.Obj(banner).Card, banner)
	if o := e.G.Obj(cp); o.Zone != state.ZBattlefield || o.ChosenColor != "R" {
		t.Fatalf("Banner copy: zone %s chosen colour %q, want the battlefield with R", o.Zone, o.ChosenColor)
	}
	if got := e.G.Obj(banner).ChosenColor; got != "W" {
		t.Fatalf("the original Banner's colour changed to %q", got)
	}
}
