package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

const eatenAliveLike = "Name:Eaten Alive Probe\nManaCost:B\nTypes:Sorcery\nK:AlternateAdditionalCost:Sac<1/Creature>:3 B\nA:SP$ ChangeZone | ValidTgts$ Creature | Origin$ Battlefield | Destination$ Exile\nOracle:x\n"

// An either-or additional cost ("As an additional cost to cast this spell,
// sacrifice a creature or pay {3}{B}", Eaten Alive) applies to a spell cast
// through a may-play permission too (CR 601.2b/f, 118.9). The may-play cast
// used to skip it: gorge cast Eaten Alive from Tinybones' stash for {B} alone
// (census pair 14).
func TestMayPlayCastPaysEitherOrAdditionalCost(t *testing.T) {
	setup := func(withFodder bool) (*Engine, state.ObjID, state.ObjID, state.ObjID) {
		e := mayPlayBase(t)
		onBoardGrant(t, e, 0, "Name:Grave Grant\nTypes:Enchantment\nS:Mode$ Continuous | Affected$ Card.YouOwn | MayPlay$ True | EffectZone$ Battlefield | AffectedZone$ Graveyard\nOracle:x\n")
		spell := graveCard(e, card(t, eatenAliveLike), 0, 0)
		victim := onBoardGrant(t, e, 1, "Name:Victim Bear\nManaCost:1 G\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")
		var fodder state.ObjID
		if withFodder {
			fodder = onBoardGrant(t, e, 0, "Name:Fodder Bear\nManaCost:1 G\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")
		}
		addMana(t, e, 0, "B")
		e.priorityRound()
		return e, spell, victim, fodder
	}
	castOption := func(e *Engine, spell state.ObjID) int {
		d := e.Pending()
		if d == nil || d.Kind != decision.KPriority {
			t.Fatalf("pending = %+v, want priority", d)
		}
		for _, o := range d.Options {
			if o.Kind == "cast" && o.Obj == spell {
				return o.Index
			}
		}
		return -1
	}

	e, spell, _, _ := setup(false)
	if idx := castOption(e, spell); idx >= 0 {
		t.Fatalf("may-play cast offered with {B} and neither a creature to sacrifice nor {3}{B}: %+v", e.Pending().Options[idx])
	}

	e, spell, victim, fodder := setup(true)
	idx := castOption(e, spell)
	if idx < 0 {
		t.Fatalf("may-play cast not offered with a creature to sacrifice: %+v", e.Pending().Options)
	}
	submitChoices(t, e, idx)
	for i := 0; i < 8; i++ {
		d := e.Pending()
		if d == nil || d.Kind == decision.KPriority {
			break
		}
		pick := 0
		for _, o := range d.Options {
			if o.Obj == victim || o.Obj == fodder {
				pick = o.Index
				if (d.Kind == decision.KTarget && o.Obj == victim) || (d.Kind != decision.KTarget && o.Obj == fodder) {
					break
				}
			}
		}
		submitChoices(t, e, pick)
	}
	fdnCensusDrain(t, e)
	if z := e.G.Obj(fodder).Zone; z != state.ZGraveyard {
		t.Fatalf("the may-play cast did not sacrifice the creature for its additional cost: Fodder Bear in %s", z)
	}
	if z := e.G.Obj(victim).Zone; z != state.ZExile {
		t.Fatalf("Victim Bear in %s, want exile", z)
	}
}
