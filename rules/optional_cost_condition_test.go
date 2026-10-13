package rules

// A bare `Condition$ OptionalCost | ConditionOptionalPaid$ True|False` gate
// (10 corpus lines over 8 files: Osseous Exhale's "If a Dragon was beheld,
// you gain 2 life", Piercing Exhale's beheld surveil, Draconic Fealty's
// beheld graveyard exile). CR 601.2b: the optional additional cost is chosen
// and paid as the spell is cast; the spell's "if [the cost] was paid" text
// reads that payment. The gate used to be classified unmodelled and failed
// CLOSED with a Note, so a paid cast never ran its rider (level-B compliance
// row TDM Osseous Exhale static#0.0). The fixture is freely authored: a
// Blight<1> optional cost (the corpus-proven optional part) whose paid rider
// gains 2 and whose unpaid rider gains 4, so the two polarities cannot be
// confused.

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

const optionalPaidGateSrc = "Name:Gilded Choice\nManaCost:W\nTypes:Sorcery\n" +
	"S:Mode$ OptionalCost | EffectZone$ All | ValidCard$ Card.Self | ValidSA$ Spell | Cost$ Blight<1> | Description$ As an additional cost to cast this spell, you may blight 1.\n" +
	"A:SP$ GainLife | Defined$ You | LifeAmount$ 1 | SubAbility$ DBPaid | SpellDescription$ You gain 1 life.\n" +
	"SVar:DBPaid:DB$ GainLife | Condition$ OptionalCost | ConditionOptionalPaid$ True | Defined$ You | LifeAmount$ 2 | SubAbility$ DBUnpaid\n" +
	"SVar:DBUnpaid:DB$ GainLife | Condition$ OptionalCost | ConditionOptionalPaid$ False | Defined$ You | LifeAmount$ 4\n" +
	"Oracle:x\n"

func optionalPaidGateLife(t *testing.T, mode string) int32 {
	t.Helper()
	e, cfg, _ := altCostEngine(t, 612, nil, []string{optionalPaidGateSrc, altBearSrc}, []string{altBearSrc})
	spell := findCardObj(t, e, 0, "Gilded Choice", state.ZHand)
	bear := findCardObj(t, e, 0, "Bear", state.ZBattlefield)
	addMana(t, e, 0, "W")
	submitChoices(t, e, castModeOption(t, e, spell, mode))
	if d := e.Pending(); mode == "optionalcost" && d != nil && d.Kind == decision.KChoose {
		choosePendingObject(t, e, bear)
	}
	passUntilStackEmpty(t, e, 40)
	if paid := e.G.Obj(bear).Counter("M1M1") == 1; paid != (mode == "optionalcost") {
		t.Fatalf("mode %q: blight paid=%v", mode, paid)
	}
	replayCheck(t, e, cfg)
	return e.G.Players[0].Life
}

func TestConditionOptionalCostPaidRunsThePaidRider(t *testing.T) {
	t.Parallel()
	if got := optionalPaidGateLife(t, "optionalcost"); got != 23 {
		t.Fatalf("paid cast: life=%d, want 23 (1 + the paid rider's 2)", got)
	}
}

func TestConditionOptionalCostDeclinedRunsTheUnpaidRider(t *testing.T) {
	t.Parallel()
	if got := optionalPaidGateLife(t, ""); got != 25 {
		t.Fatalf("declined cast: life=%d, want 25 (1 + the unpaid rider's 4)", got)
	}
}
