package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// Frenzied Goblin (the user report, 2026-10-14): "Whenever Frenzied Goblin
// attacks, you may pay {R}. If you do, target creature can't block this
// turn." -- OptionalDecider$ You over an AB$ Pump body with Cost$ R. With an
// untapped Mountain the attack used to pose FOUR asks: the target, the
// trigger_optional yes/no, a manual "Tap Mountain for mana / Done" window and
// then "Pay R / Do not pay". The pay-first model (respay.go) and the folded
// optional (optionalFoldsIntoCost) leave exactly two: the target as the
// trigger goes on the stack (CR 603.3d), then one pay election at resolution.

// frenziedGoblinAttack sets up the Goblin with one untapped Mountain and an
// opposing creature, attacks, answers the target ask with that creature and
// returns the pending pay election plus the fixture ids.
func frenziedGoblinAttack(t *testing.T) (e *Engine, election *decision.Decision, mountain, target state.ObjID) {
	t.Helper()
	reg := searchTestRegistry(t)
	e, _ = searchEngine(t, reg, "Frenzied Goblin")
	goblin := searchMoveByName(t, e, "Frenzied Goblin", state.ZBattlefield)
	e.G.Obj(goblin).SummonSick = false
	mountain = xFoldLand(t, e, "Test Mountain", "R")
	target = onBoard(t, e, 1, "Name:Test Bear\nManaCost:1 G\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")

	driveToStep(t, e, e.G.Turn, e.G.Active, state.StepDeclareAttackers)
	submitAttackersOnly(t, e, goblin)

	// Ask 1: the target, chosen as the trigger goes on the stack.
	d := passUntilNonPriority(t, e, 40)
	if d.Kind != decision.KTarget {
		t.Fatalf("ask 1 = %s %q, want the target ask", d.Kind, d.Prompt)
	}
	pick := -1
	for _, o := range d.Options {
		if o.Obj == target {
			pick = o.Index
		}
	}
	if pick < 0 {
		t.Fatalf("the target ask does not offer the opposing creature: %+v", d.Options)
	}
	submitChoices(t, e, pick)

	// Ask 2: the one pay election -- no trigger_optional yes/no and no mana
	// window in front of it.
	election = passUntilNonPriority(t, e, 40)
	if election.Kind != decision.KChoose {
		t.Fatalf("ask 2 = %s %q, want the pay election", election.Kind, election.Prompt)
	}
	var kinds []string
	for _, o := range election.Options {
		kinds = append(kinds, o.Kind)
	}
	want := []string{"trigger_cost_pay", "trigger_cost_decline", "trigger_cost_pay_manual"}
	if len(kinds) != len(want) {
		t.Fatalf("election option kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("election option kinds = %v, want %v", kinds, want)
		}
	}
	return e, election, mountain, target
}

// goblinOptionIndex returns the election's option of the given kind.
func goblinOptionIndex(t *testing.T, d *decision.Decision, kind string) int {
	t.Helper()
	for _, o := range d.Options {
		if o.Kind == kind {
			return o.Index
		}
	}
	t.Fatalf("no %s option in %+v", kind, d.Options)
	return -1
}

// goblinBlockersOffer drives to the defender's blockers declaration and
// reports whether it offers target as a blocker. A combat with no possible
// blocker poses no declaration at all, so the next turn's attackers ask
// reads as "not offered"; any other ask (a second payment ask) fails.
func goblinBlockersOffer(t *testing.T, e *Engine, target state.ObjID) bool {
	t.Helper()
	d := passUntilNonPriority(t, e, 40)
	if d.Kind == decision.KAttackers && d.Player == 1 {
		return false
	}
	if d.Kind != decision.KBlockers {
		t.Fatalf("after the election the next ask is %s %q, want the blockers declaration (no further payment ask)", d.Kind, d.Prompt)
	}
	for _, o := range d.Options {
		if o.Obj == target {
			return true
		}
	}
	return false
}

// TestFrenziedGoblinPayAutoTapsWithNoWindow is the auto route (the web's
// auto-mana surface): "Pay R" taps the Mountain through the payment planner,
// with no manual window, and the target cannot block.
func TestFrenziedGoblinPayAutoTapsWithNoWindow(t *testing.T) {
	t.Parallel()
	e, d, mountain, target := frenziedGoblinAttack(t)
	submitChoices(t, e, goblinOptionIndex(t, d, "trigger_cost_pay"))

	if !e.G.Obj(mountain).Tapped {
		t.Fatal("the auto route did not tap the Mountain")
	}
	if got := e.G.Players[0].Pool; got != (state.Mana{}) {
		t.Fatalf("pool = %v after paying, want empty (the {R} was charged)", got)
	}
	if goblinBlockersOffer(t, e, target) {
		t.Fatal("the paid trigger's target is still offered as a blocker")
	}
}

// TestFrenziedGoblinPayManualRouteTapsChosenSource is the manual route (the
// web's auto-mana-off surface): the manual window opens AFTER the election,
// the payer taps the Mountain, and the cost settles with no second election.
func TestFrenziedGoblinPayManualRouteTapsChosenSource(t *testing.T) {
	t.Parallel()
	e, d, mountain, target := frenziedGoblinAttack(t)
	submitChoices(t, e, goblinOptionIndex(t, d, "trigger_cost_pay_manual"))

	w := e.Pending()
	if w == nil || w.Kind != decision.KChoose {
		t.Fatalf("the manual route posed %+v, want the mana window", w)
	}
	tap := -1
	for _, o := range w.Options {
		if o.Kind == "activate" && o.Obj == mountain {
			tap = o.Index
		}
	}
	if tap < 0 {
		t.Fatalf("the manual window does not offer the Mountain: %+v", w.Options)
	}
	submitChoices(t, e, tap)

	if !e.G.Obj(mountain).Tapped {
		t.Fatal("the Mountain was not tapped")
	}
	if got := e.G.Players[0].Pool; got != (state.Mana{}) {
		t.Fatalf("pool = %v after paying, want empty", got)
	}
	if goblinBlockersOffer(t, e, target) {
		t.Fatal("the paid trigger's target is still offered as a blocker")
	}
}

// TestFrenziedGoblinDeclinePaysNothingAndTargetBlocks is the decline: nothing
// is tapped or paid and the target can still block.
func TestFrenziedGoblinDeclinePaysNothingAndTargetBlocks(t *testing.T) {
	t.Parallel()
	e, d, mountain, target := frenziedGoblinAttack(t)
	submitChoices(t, e, goblinOptionIndex(t, d, "trigger_cost_decline"))

	if e.G.Obj(mountain).Tapped {
		t.Fatal("a declined cost tapped the Mountain")
	}
	if got := e.G.Players[0].Pool; got != (state.Mana{}) {
		t.Fatalf("pool = %v after a decline, want empty", got)
	}
	if !goblinBlockersOffer(t, e, target) {
		t.Fatal("the declined trigger still stopped its target from blocking")
	}
}
