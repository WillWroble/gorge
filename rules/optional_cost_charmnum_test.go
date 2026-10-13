// optional_cost_charmnum_test.go -- CR 601.2b: a modal spell's mode count may
// depend on its optional additional cost, which is announced together with
// the modes. Pyrrhic Strike ("As an additional cost to cast this spell, you
// may blight 2. Choose one. If this spell's additional cost was paid, choose
// both instead.") scripts the count as CharmNum$ X over
// Count$OptionalGenericCostPaid.2.1. At the mode announcement the card is
// still in hand and the pay-time OptionalCostPaid flag is not folded yet, so
// the count must read the cast option's own election; before the seed the
// paid cast was asked "choose one".

package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

const (
	pyrrhicGiantSrc = "Name:Test Giant\nManaCost:2 R\nTypes:Creature Giant\nPT:3/3\nOracle:x\n"
	pyrrhicRelicSrc = "Name:Test Relic\nManaCost:1\nTypes:Artifact\nOracle:x\n"
)

// pyrrhicModeAsk casts Pyrrhic Strike in the given cast mode with {2}{W}
// floating and returns the CR 601.2b mode decision it poses.
func pyrrhicModeAsk(t *testing.T, seed uint64, mode string) *decision.Decision {
	t.Helper()
	e, _, _ := altCostEngine(t, seed, []string{"Pyrrhic Strike"},
		[]string{pyrrhicGiantSrc}, []string{pyrrhicRelicSrc, altBearSrc})
	spell := findCardObj(t, e, 0, "Pyrrhic Strike", state.ZHand)
	findCardObj(t, e, 0, "Test Giant", state.ZBattlefield)
	findCardObj(t, e, 1, "Test Relic", state.ZBattlefield)
	findCardObj(t, e, 1, "Bear", state.ZBattlefield)
	addMana(t, e, 0, "CCW")
	submitChoices(t, e, castModeOption(t, e, spell, mode))
	for i := 0; i < 4; i++ {
		d := e.Pending()
		if d == nil {
			t.Fatal("no decision pending mid-cast")
		}
		if d.Kind == decision.KModes {
			return d
		}
		submitChoices(t, e, d.Options[0].Index)
	}
	t.Fatal("the cast never posed its mode announcement")
	return nil
}

func TestPyrrhicStrikePaidOptionalCostChoosesBothModes(t *testing.T) {
	t.Parallel()
	d := pyrrhicModeAsk(t, 811, "optionalcost")
	if d.Min != 2 || d.Max != 2 {
		t.Fatalf("paid cast mode bounds = %d..%d, want 2..2 (CR 601.2b: the elected additional cost chooses both)", d.Min, d.Max)
	}
}

func TestPyrrhicStrikePlainCastChoosesOneMode(t *testing.T) {
	t.Parallel()
	d := pyrrhicModeAsk(t, 812, "")
	if d.Min != 1 || d.Max != 1 {
		t.Fatalf("plain cast mode bounds = %d..%d, want 1..1 (the election seed must not leak into the plain cast)", d.Min, d.Max)
	}
}
