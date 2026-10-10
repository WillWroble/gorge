package botpolicy

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

// loyaltyPriority offers one walker's abilities (labels carry no digit, as
// the engine's loyalty labels do) in front of pass.
func loyaltyPriority(facts ...decision.Loyalty) *decision.Decision {
	d := &decision.Decision{Seq: 1, Player: 0, Kind: decision.KPriority, Min: 1, Max: 1}
	for i := range facts {
		f := facts[i]
		d.Options = append(d.Options, decision.Option{Index: i, Kind: "ability", Obj: 50, Ability: i, Label: "Walker: do a thing", Loyalty: &f})
	}
	d.Options = append(d.Options, decision.Option{Index: len(facts), Kind: "pass"})
	return d
}

func loyaltyPick(t *testing.T, b Board, facts ...decision.Loyalty) int {
	t.Helper()
	d := loyaltyPriority(facts...)
	in := Decide(b, d, rng(1))
	if err := d.Validate(in); err != nil {
		t.Fatalf("intent %+v failed Validate: %v", in, err)
	}
	return in.Choices[0]
}

// TestLoyaltyRanking pins A7: the pick follows the loyalty fact, not option
// order. Each case has a first option that the old label-tie rule would take.
func TestLoyaltyRanking(t *testing.T) {
	foe := Board{IsMain: true, MyTurn: true,
		Creatures: TableOf(map[state.ObjID]Creature{7: {Power: 3, Toughness: 3, Controller: 1}})}
	empty := Board{IsMain: true, MyTurn: true}
	plus, zero := decision.Loyalty{Delta: 1}, decision.Loyalty{}
	kill := decision.Loyalty{Delta: -3, Removal: true}
	draw := decision.Loyalty{Delta: -2}
	ult := decision.Loyalty{Delta: -7, Ultimate: true, Dies: true}
	x := decision.Loyalty{X: true}

	if got := loyaltyPick(t, foe, plus, kill); got != 1 {
		t.Errorf("plus vs removal with an opposing creature: picked %d, want the removal (1)", got)
	}
	if got := loyaltyPick(t, empty, plus, kill); got != 0 {
		t.Errorf("plus vs removal with no opposing creature: picked %d, want the plus (0)", got)
	}
	if got := loyaltyPick(t, foe, zero, plus, ult); got != 2 {
		t.Errorf("affordable ultimate: picked %d, want the ultimate (2)", got)
	}
	if got := loyaltyPick(t, empty, draw, zero, plus); got != 2 {
		t.Errorf("plus vs zero vs minus: picked %d, want the plus (2)", got)
	}
	if got := loyaltyPick(t, empty, x, draw, decision.Loyalty{Delta: -1}); got != 2 {
		t.Errorf("minuses only: picked %d, want the smallest minus (2)", got)
	}
	// A game restart (a draw in this engine) is never taken, even over pass.
	if got := loyaltyPick(t, foe, decision.Loyalty{Delta: -14, Ultimate: true, Restart: true}); got != 1 {
		t.Errorf("restart ultimate alone: picked %d, want pass (1)", got)
	}
	// A removal that kills the walker ranks below a plus.
	if got := loyaltyPick(t, foe, decision.Loyalty{Delta: -3, Removal: true, Dies: true}, plus); got != 1 {
		t.Errorf("walker-killing removal vs plus: picked %d, want the plus (1)", got)
	}
}
