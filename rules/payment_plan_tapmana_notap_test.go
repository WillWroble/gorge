package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/rules/pay"
	"github.com/adams-shaun/gorge/state"
)

// A mana ability whose cost does not tap never fires a TapsForMana trigger:
// a sacrifice-only {C} source under Forsaken Monument stays {C}, not {C}{C}.
func TestPaymentPlanTapManaFoldIgnoresNoTapCost(t *testing.T) {
	t.Parallel()
	e, _, _ := newFixtureDeck(t, 9916, interferenceOneInstant)
	spawn := onBoard(t, e, 0, "Name:Sac Spawn\nTypes:Creature Eldrazi Spawn\nPT:0/1\nA:AB$ Mana | Cost$ Sac<1/CARDNAME> | Produced$ C | SpellDescription$ Add {C}.\nOracle:x\n")
	dork := onBoard(t, e, 0, tapManaDork)
	e.G.Obj(dork).SummonSick = false
	tapManaTriggerPrecondition(t, e, lrCorpus(t, e, "Forsaken Monument"))
	if got := interferenceAltMana(t, e, dork); got != (state.Mana{state.MC: 2}) {
		t.Fatalf("precondition: tapping dork under Monument = %v, want {C}{C}", got)
	}
	var alts []pay.Alt
	for _, u := range e.paymentPlanManaUnits(0) {
		if u.ID == spawn {
			alts = pay.PaymentPlanUnitAlternatives(asPayer(e), u)
		}
	}
	if len(alts) != 1 || alts[0].Tier != pay.TierLastResort {
		t.Fatalf("spawn alternatives = %+v, want one last-resort (sacrifice) alternative", alts)
	}
	if alts[0].Mana != (state.Mana{state.MC: 1}) || alts[0].Activation.Produces != (decision.ManaAmount{state.MC: 1}) {
		t.Fatalf("sac-only spawn under Monument = %v / %v, want {C} (no tap, no trigger)", alts[0].Mana, alts[0].Activation.Produces)
	}
}
