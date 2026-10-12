package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/state"
)

// TestUneasyAllianceExilesEnchantedAfterSacrifice pins Defined$ Enchanted
// resolving off the Aura's last bearer once the Aura itself was sacrificed
// as the activation cost ("{5}, Sacrifice this Aura: Exile enchanted
// creature"): the move fold has cleared AttachedTo by resolution, so the
// resolver must fall back to LastBearer (CR 608.2h last known information).
func TestUneasyAllianceExilesEnchantedAfterSacrifice(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	e, ids := attachedToFixture(t, reg, 9411, []string{"Uneasy Alliance"}, []string{"Hill Giant"}, nil)
	giant, aura := ids["Hill Giant"], ids["Uneasy Alliance"]

	addMana(t, e, 0, "1W")
	castNamed(t, e, "Uneasy Alliance")
	attachDrain(t, e, 40)
	ao := e.G.Obj(aura)
	if ao == nil || ao.Zone != state.ZBattlefield || ao.AttachedTo != giant {
		t.Fatalf("precondition: Aura must be attached to the Giant on the battlefield, got %+v", ao)
	}

	addMana(t, e, 0, "CCCCC")
	submitChoices(t, e, abilityOption(t, e, aura, 0).Index)
	attachDrain(t, e, 40)

	if ao := e.G.Obj(aura); ao.Zone != state.ZGraveyard {
		t.Fatalf("Aura zone = %s, want graveyard (sacrificed as a cost)", ao.Zone)
	}
	if gz := e.G.Obj(giant).Zone; gz != state.ZExile {
		t.Fatalf("Hill Giant zone = %s, want exile", gz)
	}
}

// attachLKICarriers is every corpus card measured on 2026-10-12 to carry an
// activated ability whose cost sacrifices the source itself AND whose
// Defined$ is Enchanted|Equipped|AttachedTo (the bearer is read after the
// source left the battlefield):
//
//	/usr/bin/grep -rlE '^A:.*Sac<1/CARDNAME[^>]*>.*Defined\$ (Enchanted|Equipped|AttachedTo)\b|^A:.*Defined\$ (Enchanted|Equipped|AttachedTo)\b.*Sac<1/CARDNAME' .cards/cardsfolder | wc -l  ==  10
//
// All ride effects ResolveDefined's definedSpecEquipped LastBearer fallback.
var attachLKICarriers = []string{
	"Briar Shield", "Candletrap", "Carapace", "Choking Restraints",
	"Path to Redemption", "Phantom Wings", "Spiral into Solitude", "Stamina",
	"Thrull Retainer", "Uneasy Alliance",
}

// TestAttachLKICarrierCensus ratchets the class: a carrier that drops the
// shape (or a script that gains one) must be re-classified here.
func TestAttachLKICarrierCensus(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	for _, name := range attachLKICarriers {
		c := searchCorpusCard(t, reg, name)
		found := false
		for _, f := range c.Faces {
			for _, sa := range f.Abilities {
				switch sa.ParamStr(cards.PKDefined) {
				case "Enchanted", "Equipped", "AttachedTo":
					if strings.Contains(sa.ParamStr(cards.PKCost), "Sac<1/CARDNAME") {
						found = true
					}
				}
			}
		}
		if !found {
			t.Errorf("%s no longer carries a self-sacrifice + Defined$ Enchanted|Equipped|AttachedTo ability", name)
		}
	}
	if len(attachLKICarriers) != 10 {
		t.Errorf("carrier list has %d entries, want 10", len(attachLKICarriers))
	}
}
