package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

// TestLoyaltyOptionFacts pins decision.Option.Loyalty: a planeswalker's
// offered abilities carry the net loyalty change, the ultimate marker and the
// removal classification read from the compiled cost/ability, and an
// ordinary activated ability carries none.
func TestLoyaltyOptionFacts(t *testing.T) {
	t.Parallel()
	walker := card(t, "Name:Fact Walker\nTypes:Planeswalker Test\nLoyalty:4\n"+
		"A:AB$ GainLife | Cost$ AddCounter<1/LOYALTY> | LifeAmount$ 2 | Planeswalker$ True | SpellDescription$ Gain 2.\n"+
		"A:AB$ Destroy | Cost$ SubCounter<2/LOYALTY> | ValidTgts$ Creature | TgtPrompt$ Select target creature | Planeswalker$ True | SpellDescription$ Destroy target creature.\n"+
		"A:AB$ GainLife | Cost$ SubCounter<4/LOYALTY> | LifeAmount$ 20 | Planeswalker$ True | Ultimate$ True | SpellDescription$ Gain 20.\n"+
		"A:AB$ RestartGame | Cost$ SubCounter<3/LOYALTY> | Planeswalker$ True | Ultimate$ True | SpellDescription$ Restart.\n"+
		"A:AB$ ChangeZoneAll | Cost$ SubCounter<1/LOYALTY> | Origin$ Battlefield | Destination$ Exile | ChangeType$ Creature | Planeswalker$ True | SpellDescription$ Exile all creatures.\n"+
		"A:AB$ ChangeZoneAll | Cost$ SubCounter<1/LOYALTY> | Origin$ Graveyard | Destination$ Exile | ChangeType$ Card | Planeswalker$ True | SpellDescription$ Exile all graveyards.\n"+
		"Oracle:x\n")
	victim := card(t, "Name:Fact Victim\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")
	e, _ := tokenReplGameSeats(t, 301, []*cards.Card{walker}, []*cards.Card{victim})
	id := moveSeededCard(t, e, 0, walker, state.ZBattlefield)
	moveSeededCard(t, e, 1, victim, state.ZBattlefield)
	if got := e.G.Obj(id).Counter("LOYALTY"); got != 4 {
		t.Fatalf("precondition: walker loyalty = %d, want 4", got)
	}
	addMana(t, e, 0, "")
	plus := abilityOption(t, e, id, 0).Loyalty
	minus := abilityOption(t, e, id, 1).Loyalty
	ult := abilityOption(t, e, id, 2).Loyalty
	restart := abilityOption(t, e, id, 3).Loyalty
	if restart == nil || !restart.Restart || minus.Restart || ult.Restart {
		t.Errorf("RestartGame fact = %+v (minus %+v, ult %+v), want Restart only on the restart option", restart, minus, ult)
	}
	if plus == nil || minus == nil || ult == nil {
		t.Fatalf("loyalty options lack the fact: plus=%v minus=%v ult=%v", plus, minus, ult)
	}
	if *plus != (decision.Loyalty{Delta: 1}) {
		t.Errorf("[+1] fact = %+v, want Delta 1 only", *plus)
	}
	if *minus != (decision.Loyalty{Delta: -2, Removal: true}) {
		t.Errorf("[-2] Destroy fact = %+v, want Delta -2 Removal", *minus)
	}
	if *ult != (decision.Loyalty{Delta: -4, Ultimate: true, Dies: true}) {
		t.Errorf("[-4] ultimate fact = %+v, want Delta -4 Ultimate Dies", *ult)
	}
	// ChangeZoneAll is removal only in its battlefield-leaving shape: a
	// graveyard/library exile leaves every battlefield permanent untouched.
	exileAll := abilityOption(t, e, id, 4).Loyalty
	exileGrave := abilityOption(t, e, id, 5).Loyalty
	if exileAll == nil || !exileAll.Removal {
		t.Errorf("ChangeZoneAll Battlefield->Exile fact = %+v, want Removal", exileAll)
	}
	if exileGrave == nil || exileGrave.Removal {
		t.Errorf("ChangeZoneAll Graveyard->Exile fact = %+v, want not Removal", exileGrave)
	}
}
