package rules

// Teval, Arbiter of Virtue's "Spells you cast have delve" is a layer-6
// AddKeyword$ Delve grant scoped to AffectedZone$ Stack over
// Card.YouCtrl+wasCast. CR 702.66a: "Delve is a static ability that
// functions while the spell with delve is on the stack"; CR 601.2f-h price
// and pay the spell's cost while it is on the stack, so the grant reaches
// the cost the cast pays. The delve reads used to look only at the card's
// current (hand) zone, where the stack-scoped grant never applies, so a
// spell short of mana by what its delve covers was never offered
// (level-B compliance row TDM Teval static#0.0).

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

func tevalDelveEngine(t *testing.T, teval bool) (*Engine, state.ObjID, state.ObjID) {
	t.Helper()
	e := handEngine(t, corpusAlternativeCard(t, "Teval, Arbiter of Virtue"))
	if teval {
		for _, o := range e.G.Zone(state.ZHand, 0) {
			e.emit(events.Event{Kind: events.MoveZone, Obj: o, From: state.ZHand, To: state.ZBattlefield})
		}
	}
	gy := e.G.AddObject(card(t, "Name:Husk\nManaCost:G\nTypes:Creature Husk\nPT:1/1\nOracle:x\n"), 0)
	e.emit(events.Event{Kind: events.MoveZone, Obj: gy.ID, From: state.ZLibrary, To: state.ZGraveyard})
	spell := e.G.AddObject(card(t, "Name:Ogre\nManaCost:3 R\nTypes:Creature Ogre\nPT:3/3\nOracle:x\n"), 0)
	e.G.SetZone(state.ZHand, 0, append(e.G.Zone(state.ZHand, 0), spell.ID))
	return e, spell.ID, gy.ID
}

func offeredCast(e *Engine, id state.ObjID) int {
	d := e.Pending()
	if d == nil {
		return -1
	}
	for _, o := range d.Options {
		if o.Kind == "cast" && o.Obj == id {
			return o.Index
		}
	}
	return -1
}

func TestGrantedDelveReachesTheCastSpell(t *testing.T) {
	t.Parallel()
	e, spell, gy := tevalDelveEngine(t, true)
	// {3}{R} with {2}{R} in the pool: one graveyard card delved pays the rest.
	addMana(t, e, 0, "RCC")
	opt := offeredCast(e, spell)
	if opt < 0 {
		t.Fatalf("CR 702.66a: the spell with granted delve is not offered: %+v", e.Pending().Options)
	}
	submitChoices(t, e, opt)
	d := e.Pending()
	if d == nil || d.Kind != decision.KChoose {
		t.Fatalf("no delve ask: %+v", d)
	}
	pick := -1
	for _, o := range d.Options {
		if o.Obj == gy {
			pick = o.Index
		}
	}
	if pick < 0 {
		t.Fatalf("delve ask does not offer the graveyard card: %+v", d.Options)
	}
	submitChoices(t, e, pick)
	passUntilStackEmpty(t, e, 20)
	if z := e.G.Obj(spell).Zone; z != state.ZBattlefield {
		t.Fatalf("Ogre zone=%s, want battlefield (delve paid {1})", z)
	}
	if z := e.G.Obj(gy).Zone; z != state.ZExile {
		t.Fatalf("delved card zone=%s, want exile", z)
	}
}

func TestGrantedDelveAbsentWithoutTheGrant(t *testing.T) {
	t.Parallel()
	e, spell, _ := tevalDelveEngine(t, false)
	addMana(t, e, 0, "RCC")
	if opt := offeredCast(e, spell); opt >= 0 {
		t.Fatalf("a {3}{R} spell without delve is offered on {2}{R}")
	}
}
