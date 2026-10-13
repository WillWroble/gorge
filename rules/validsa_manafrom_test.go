package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"

	"github.com/adams-shaun/gorge/state"
)

// Spell.ManaFrom<Tag> as a SpellCast ValidSA$ (Smaug, Wicked Worm; Mastermind
// Plum; Alchemist's Talent) reads the same cast spend window as the
// card-level "CastSa Spell.ManaFrom<Tag>" spelling.

func smaugCastEngine(t *testing.T, seed uint64) (*Engine, state.ObjID, state.ObjID) {
	t.Helper()
	e, _ := cascadeTestEngineFiller(t, seed, "Grizzly Bears", nil, []string{"Smaug, Wicked Worm"}, "Grizzly Bears")
	passUntilStackEmpty(t, e, 20) // Smaug's own ETB trigger
	var smaug state.ObjID
	for _, id := range e.G.Zone(state.ZBattlefield, 0) {
		if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Smaug, Wicked Worm" {
			smaug = id
		}
	}
	if smaug == 0 {
		t.Fatal("Smaug is not on the battlefield")
	}
	for _, tr := range e.G.Obj(smaug).Face().Triggers {
		if v, ok := tr.Param(cards.PKValidSA); ok && v == "Spell.ManaFromTreasure" {
			return e, smaug, searchMoveByName(t, e, "Grizzly Bears", state.ZHand)
		}
	}
	t.Fatal("Smaug carries no ValidSA$ Spell.ManaFromTreasure trigger")
	return nil, 0, 0
}

func TestSmaugTriggersWhenTreasureManaPaysCast(t *testing.T) {
	t.Parallel()
	e, _, bears := smaugCastEngine(t, 9401)
	life, hand := e.G.Players[0].Life, len(e.G.Zone(state.ZHand, 0))
	treasureUnit(t, e)
	submitCastOption(t, e, bears)
	passUntilStackEmpty(t, e, 60)
	if got := e.G.Players[0].Life; got != life-1 {
		t.Fatalf("life = %d, want %d (Smaug's lose-1 after a Treasure-mana cast)", got, life-1)
	}
	// Bears left the hand (-1) and Smaug drew a card (+1).
	if got := len(e.G.Zone(state.ZHand, 0)); got != hand {
		t.Fatalf("hand = %d, want %d", got, hand)
	}
}

func TestSmaugSilentWhenPlainManaPaysCast(t *testing.T) {
	t.Parallel()
	e, _, bears := smaugCastEngine(t, 9402)
	life, hand := e.G.Players[0].Life, len(e.G.Zone(state.ZHand, 0))
	addMana(t, e, 0, "GG")
	submitCastOption(t, e, bears)
	passUntilStackEmpty(t, e, 60)
	if o := e.G.Obj(bears); o.Zone != state.ZBattlefield {
		t.Fatalf("Bears resolved to %s, want the battlefield", o.Zone)
	}
	if got := e.G.Players[0].Life; got != life {
		t.Fatalf("life = %d, want %d (no Treasure mana was spent)", got, life)
	}
	if got := len(e.G.Zone(state.ZHand, 0)); got != hand-1 {
		t.Fatalf("hand = %d, want %d", got, hand-1)
	}
}
