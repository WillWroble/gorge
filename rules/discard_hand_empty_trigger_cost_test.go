// discard_hand_empty_trigger_cost_test.go — "discard your hand" as a
// triggered ability's optional cost is payable with an EMPTY hand: discarding
// zero cards needs no resource the payer lacks (CR 118.3), as Byway
// Barterer's ruling spells out ("You may discard your hand even if it has no
// cards in it. If you do, you'll draw two cards."). The window's offer gate
// used to read the token's count (Discard<1/Hand>) as "needs a card" and
// posed a decline-only ask, while the cast path (pay.DiscardCostPayable)
// already treated the Hand spec as always payable. Byway Barterer is in no
// legacy golden deck, so no chain head moves.

package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

func TestBywayBartererDiscardsEmptyHandToDraw(t *testing.T) {
	t.Parallel()
	e, cfg, _ := altCostEngine(t, 741, []string{"Byway Barterer", "Inspiration"}, nil, []string{altBearSrc})
	findCardObj(t, e, 0, "Byway Barterer", state.ZBattlefield)
	spell := findCardObj(t, e, 0, "Inspiration", state.ZHand)
	for _, id := range append([]state.ObjID(nil), e.G.Zone(state.ZHand, 0)...) {
		if id != spell {
			e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZHand, To: state.ZLibrary})
		}
	}
	e.pending = nil
	e.priorityRound()
	addMana(t, e, 0, "CCCU")
	submitChoices(t, e, castModeOption(t, e, spell, ""))
	paid := false
	for i := 0; i < 40 && (len(e.G.Stack) > 0 || e.Pending().Kind != decision.KPriority); i++ {
		d := e.Pending()
		switch d.Kind {
		case decision.KPriority:
			passOnceP(t, e)
		case decision.KTarget:
			for _, o := range d.Options {
				if o.Player == 0 && o.Obj == 0 {
					submitChoices(t, e, o.Index)
					break
				}
			}
		case decision.KChoose:
			pick := -1
			for _, o := range d.Options {
				if o.Kind == "trigger_cost_pay" {
					pick = o.Index
				}
			}
			if pick < 0 {
				t.Fatalf("the empty-hand discard was not offered as payable: %+v", d.Options)
			}
			if got := len(e.G.Zone(state.ZHand, 0)); got != 0 {
				t.Fatalf("precondition: hand holds %d cards at the ask, want 0", got)
			}
			paid = true
			submitChoices(t, e, pick)
		default:
			t.Fatalf("unexpected ask %+v", d)
		}
	}
	if !paid {
		t.Fatal("the expend-4 trigger's cost ask was never posed")
	}
	// Two from the trigger, two from Inspiration.
	if got := len(e.G.Zone(state.ZHand, 0)); got != 4 {
		t.Fatalf("hand holds %d cards, want 4 (2 + Inspiration's 2)", got)
	}
	replayCheck(t, e, cfg)
}
