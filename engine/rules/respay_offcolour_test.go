package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// TestTriggeredPayNotOfferedWithoutTheColour (dirty-dirac pair 589, Flamewake
// Phoenix with only Tranquil Cove untapped): a triggered "you may pay {R}"
// whose payer's one untapped source makes {W} or {U} offers decline only
// (CR 118.3). PotentialMana prices the Cove's "Combo W U" unbounded in every
// colour, which used to offer "Pay R"; answering it opened a window that could
// only fail.
func TestTriggeredPayNotOfferedWithoutTheColour(t *testing.T) {
	reg := searchTestRegistry(t)
	e, _ := searchEngine(t, reg, "Frenzied Goblin")
	goblin := searchMoveByName(t, e, "Frenzied Goblin", state.ZBattlefield)
	e.G.Obj(goblin).SummonSick = false
	xFoldLand(t, e, "Test Cove", "Combo W U")
	target := onBoard(t, e, 1, "Name:Test Bear\nManaCost:1 G\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")

	driveToStep(t, e, e.G.Turn, e.G.Active, state.StepDeclareAttackers)
	submitAttackersOnly(t, e, goblin)
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
	submitChoices(t, e, pick)

	election := passUntilNonPriority(t, e, 40)
	if election.Kind != decision.KChoose {
		t.Fatalf("ask 2 = %s %q, want the pay election", election.Kind, election.Prompt)
	}
	var kinds []string
	for _, o := range election.Options {
		kinds = append(kinds, o.Kind)
	}
	if len(kinds) != 1 || kinds[0] != "trigger_cost_decline" {
		t.Fatalf("election option kinds = %v, want [trigger_cost_decline] (no red source)", kinds)
	}
}
