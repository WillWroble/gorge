package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// Garruk's Uprising, from the card text: its ETB draw is gated by an
// intervening "if you control a creature with power 4 or greater" (CR
// 603.4), creatures you control have trample (a layer-6 grant, CR 613.1f),
// and each 4-power creature entering under your control draws a card.
func TestGarruksUprisingDrawsAndGrantsTrample(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Garruk's Uprising", "Garruk's Uprising", "Grizzly Bears", "Serra Angel")
	bears := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	hand := func() int { return len(e.G.Zone(state.ZHand, 0)) }
	settle := func() {
		e.pending = nil
		e.Advance()
		fdnCensusDrain(t, e)
	}

	uprising := moveByName(t, e, 0, "Garruk's Uprising", state.ZBattlefield)
	before := hand()
	settle()
	if got := hand(); got != before {
		t.Fatalf("Uprising entered with only a 2/2: hand %d -> %d, want no draw", before, got)
	}
	if !e.HasKeyword(bears, "Trample") {
		t.Fatalf("Grizzly Bears lacks trample under Garruk's Uprising: %v", e.Keywords(bears))
	}

	angel := moveByName(t, e, 0, "Serra Angel", state.ZBattlefield)
	before = hand()
	settle()
	if got := hand(); got != before+1 {
		t.Fatalf("a 4/4 entered under Garruk's Uprising: hand %d -> %d, want one draw", before, got)
	}
	if !e.HasKeyword(angel, "Trample") {
		t.Fatalf("Serra Angel lacks trample under Garruk's Uprising: %v", e.Keywords(angel))
	}

	// A second Uprising entering with the 4/4 present: its own ETB draws,
	// and it does not see itself as a 4-power creature entering.
	moveByName(t, e, 0, "Garruk's Uprising", state.ZBattlefield)
	before = hand()
	settle()
	if got := hand(); got != before+1 {
		t.Fatalf("second Uprising entered with a 4/4 in play: hand %d -> %d, want one draw", before, got)
	}
	_ = uprising
}
