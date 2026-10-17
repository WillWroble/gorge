package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
)

// A scenario's abstract "yes"/"no" answers an optional ask whose options
// are labelled by what they do (four-yay cmd/xmageport: XMage's setChoice(true)).
func TestOracleYesNoMatchesLabelledOptionalAsk(t *testing.T) {
	r := &oracleRun{}
	pick := func(d *decision.Decision, p string) int {
		t.Helper()
		i, err := r.matchPick(d, p, map[int]bool{})
		if err != nil {
			t.Fatal(err)
		}
		return i
	}
	payOrNot := &decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{
		{Index: 0, Kind: "choice", Label: "Exile 1 card"}, {Index: 1, Kind: "choice", Label: "Do not pay"}}}
	if got := pick(payOrNot, "yes"); got != 0 {
		t.Errorf("yes on pay/do-not-pay = %d, want 0", got)
	}
	if got := pick(payOrNot, "no"); got != 1 {
		t.Errorf("no on pay/do-not-pay = %d, want 1", got)
	}
	trigCost := &decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{
		{Index: 0, Kind: "trigger_cost_pay", Label: "Sacrifice 1 creature"}, {Index: 1, Kind: "trigger_cost_decline", Label: "Do not pay"}}}
	if got := pick(trigCost, "yes"); got != 0 {
		t.Errorf("yes on trigger cost = %d, want 0", got)
	}
	if got := pick(trigCost, "no"); got != 1 {
		t.Errorf("no on trigger cost = %d, want 1", got)
	}
	mayCast := &decision.Decision{Kind: decision.KModes, Min: 0, Max: 1, Options: []decision.Option{
		{Index: 0, Kind: "mode", Label: "Play Lava Spike"}}}
	if got := pick(mayCast, "yes"); got != 0 {
		t.Errorf("yes on a lone optional mode = %d, want 0", got)
	}
	// Not an accept/decline ask: yes matches nothing.
	block := &decision.Decision{Kind: decision.KBlockers, Min: 0, Max: 1, Options: []decision.Option{
		{Index: 0, Kind: "blocker", Label: "Llanowar Elves blocks Osseous Sticktwister"}}}
	if got := pick(block, "yes"); got != -1 {
		t.Errorf("yes on a blockers declaration = %d, want -1", got)
	}
	twoWays := &decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{
		{Index: 0, Kind: "choice", Label: "Pay U"}, {Index: 1, Kind: "choice", Label: "Pay 2 life"}}}
	if got := pick(twoWays, "yes"); got != -1 {
		t.Errorf("yes on a two-way payment = %d, want -1", got)
	}
	card := &decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{
		{Index: 0, Kind: "permanent", Label: "Northern Paladin (a)"}, {Index: 1, Kind: "permanent", Label: "Grizzly Bears (a)"}}}
	if got := pick(card, "yes"); got != -1 {
		t.Errorf("a card named No... is not a decline: yes = %d, want -1", got)
	}
}
