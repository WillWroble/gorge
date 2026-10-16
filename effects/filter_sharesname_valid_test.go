package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/state"
)

// TestSharesNameWithValidSweepsTheBattlefield pins Forge's battlefield-sweep
// referent `sharesNameWith Valid <filter>` (Key to the Side-Door's discard
// cost "Discard a legendary card with the same name as a legendary permanent
// you control"): a card in hand matches when some battlefield permanent
// matching the inner filter shares its name (CR 201.2, the name comparison),
// and fails when none does. Before the referent was classified the token
// stayed unknown and failed closed, so the ability was never offered.
func TestSharesNameWithValidSweepsTheBattlefield(t *testing.T) {
	g, id := board(t)
	c, d := cards.ParseBytes("t.txt", []byte("Name:Walker\nManaCost:2 U\nTypes:Legendary Planeswalker Test\nOracle:x\n"))
	if len(d) != 0 {
		t.Fatalf("diags: %v", d)
	}
	c.Link()
	inHand := g.AddObject(c, 0)
	inHand.Zone = state.ZHand
	g.SetZone(state.ZHand, 0, append(g.Zone(state.ZHand, 0), inHand.ID))

	const spec = "Card.Legendary+sharesNameWith Valid Permanent.Legendary+YouCtrl"
	if un := UnknownPredicates(spec); len(un) != 0 {
		t.Fatalf("UnknownPredicates(%q) = %v, want empty", spec, un)
	}
	if !MatchesSpecFrom(g, spec, inHand.ID, 0, id["myBear"]) {
		t.Fatalf("a legendary hand card named like a legendary permanent did not match %q", spec)
	}
	// A legendary card whose name no legendary permanent shares fails.
	if MatchesSpecFrom(g, "Card.sharesNameWith Valid Permanent.Legendary", id["myBear"], 0, id["myBear"]) {
		t.Fatalf("Bear matched a sweep over legendary permanents (only Walker is legendary)")
	}
	// The sweep excludes the candidate itself: the battlefield Walker shares
	// a name only with itself among legendary permanents... and the hand copy
	// is not on the battlefield, so it is no referent.
	if MatchesSpecFrom(g, "Card.sharesNameWith Valid Permanent.Legendary", id["myWalker"], 0, id["myBear"]) {
		t.Fatalf("the battlefield Walker matched its own name through the sweep")
	}
	// Remove the battlefield Walker: the hand copy no longer matches.
	g.Obj(id["myWalker"]).Zone = state.ZGraveyard
	if MatchesSpecFrom(g, spec, inHand.ID, 0, id["myBear"]) {
		t.Fatalf("the hand Walker matched with no legendary Walker on the battlefield")
	}
}
