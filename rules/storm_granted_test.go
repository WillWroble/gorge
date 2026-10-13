package rules

// Prismari, the Inspiration's "Instant and sorcery spells you cast have
// storm" is a layer-6 AddKeyword$ Storm grant scoped to AffectedZone$ Stack.
// CR 702.40a: "Storm is a triggered ability that functions on the stack.
// 'Storm' means 'When you cast this spell, copy it for each other spell that
// was cast before it this turn.'" The printed K:Storm expansion only covered
// printed lines, so a granted Storm put no trigger on the stack (level-B
// compliance row SOS Prismari static#0.0).

import (
	"github.com/adams-shaun/gorge/cards"
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

const stormProbeSpell = "Name:Mend\nManaCost:R\nTypes:Instant\nA:SP$ GainLife | Defined$ You | LifeAmount$ 1 | SpellDescription$ You gain 1 life.\nOracle:You gain 1 life.\n"

func castMendTwice(t *testing.T, prismari bool) (*Engine, int) {
	t.Helper()
	e := handEngine(t, corpusAlternativeCard(t, "Prismari, the Inspiration"), card(t, stormProbeSpell), card(t, stormProbeSpell))
	hand := append([]state.ObjID(nil), e.G.Zone(state.ZHand, 0)...)
	if prismari {
		e.emit(events.Event{Kind: events.MoveZone, Obj: hand[0], From: state.ZHand, To: state.ZBattlefield})
	}
	spells := hand[1:]
	addMana(t, e, 0, "RR")
	triggers := 0
	for _, id := range spells {
		opt := offeredCast(e, id)
		if opt < 0 {
			t.Fatalf("Mend not offered")
		}
		submitChoices(t, e, opt)
		for _, sid := range e.G.Stack {
			if so := e.G.Obj(sid); so != nil && so.Ability != nil && so.Source == id {
				triggers++
			}
		}
		passUntilStackEmpty(t, e, 20)
	}
	return e, triggers
}

func TestGrantedStormCopiesForEachEarlierSpell(t *testing.T) {
	t.Parallel()
	e, triggers := castMendTwice(t, true)
	if triggers != 2 {
		t.Fatalf("CR 702.40a: %d storm triggers on the stack, want one per granted cast", triggers)
	}
	// First Mend: storm count 0. Second Mend: one copy. 1 + 1 + 1 life.
	if life := e.G.Players[0].Life; life != 23 {
		t.Fatalf("life=%d, want 23 (two casts plus one storm copy)", life)
	}
}

func TestGrantedStormAbsentWithoutTheGrant(t *testing.T) {
	t.Parallel()
	e, triggers := castMendTwice(t, false)
	if triggers != 0 {
		t.Fatalf("%d storm triggers without the grant", triggers)
	}
	if life := e.G.Players[0].Life; life != 22 {
		t.Fatalf("life=%d, want 22", life)
	}
}

// TestGrantedStormReplaysFromTheLog: the granted trigger's body crosses the
// log as the __kwStorm: payload, so a log-only replay mints the identical
// ability and copy.
func TestGrantedStormReplaysFromTheLog(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	prismari := searchCorpusCard(t, reg, "Prismari, the Inspiration")
	mend := card(t, stormProbeSpell)
	e, cfg := demonstrateEngine(t, reg, []*cards.Card{prismari, mend, mend}, nil)
	demonstrateMove(t, e, 0, "Prismari, the Inspiration", state.ZBattlefield)
	var spells []state.ObjID
	for _, id := range e.G.Zone(state.ZLibrary, 0) {
		if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Mend" {
			spells = append(spells, id)
		}
	}
	for _, id := range e.G.Zone(state.ZHand, 0) {
		if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Mend" {
			spells = append(spells, id)
		}
	}
	if len(spells) != 2 {
		t.Fatalf("found %d Mends, want 2", len(spells))
	}
	for _, id := range spells {
		if o := e.G.Obj(id); o.Zone != state.ZHand {
			e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: o.Zone, To: state.ZHand})
		}
	}
	e.pending = nil
	e.priorityRound()
	addMana(t, e, 0, "RR")
	for _, id := range spells {
		opt := offeredCast(e, id)
		if opt < 0 {
			t.Fatalf("Mend not offered")
		}
		submitChoices(t, e, opt)
		drainStack(t, e, 20)
	}
	if life := e.G.Players[0].Life; life != 23 {
		t.Fatalf("life=%d, want 23 (two casts plus one storm copy)", life)
	}
	replayCheck(t, e, cfg)
}
