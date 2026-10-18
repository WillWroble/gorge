package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// recastCrafterScript is a craft card whose CAST carries a mandatory
// material-exile additional cost (the A:SP$ PermanentCreature line IS the
// cast of the creature -- beginCast folds the SP line's Cost$ non-mana
// parts into the cast cost through pay.WithSpellAbilityExtras), so
// payCast's craftExiledMaterials/rememberCraftMaterials path remembers the
// exiled card on the spell before it resolves onto the battlefield.
const recastCrafterScript = "Name:Recast Crafter\n" +
	"ManaCost:0\n" +
	"Types:Creature Zombie Horror\n" +
	"PT:4/5\n" +
	"K:Craft:0 ExileCtrlOrGrave<1/Artifact.Other>\n" +
	"AlternateMode:DoubleFaced\n" +
	"A:SP$ PermanentCreature | Cost$ ExileCtrlOrGrave<1/Creature>\n" +
	"Oracle:As an additional cost to cast this spell, exile a creature card from your graveyard or a creature you control.\n" +
	"\n" +
	"ALTERNATE\n" +
	"Name:Recast Crafter Awakened\n" +
	"ManaCost:no cost\n" +
	"Types:Artifact Creature\n" +
	"PT:3/3\n" +
	"Oracle:x\n"

const recastCastMaterialSrc = "Name:Cast Material\nManaCost:1 G\nTypes:Creature Bear\nPT:2/2\nOracle:x\n"
const recastSecondMaterialSrc = "Name:Second Material\nManaCost:1 G\nTypes:Creature Bear\nPT:2/2\nOracle:x\n"
const recastCraftMaterialSrc = "Name:Craft Material\nManaCost:0\nTypes:Artifact\nOracle:x\n"

// rememberedObjsOf lists the object ids in o's persistent Remembered list,
// in order. Player entries (an unrelated effect's remembered player) are
// not part of any assertion here, so they are skipped.
func rememberedObjsOf(o *state.Object) []state.ObjID {
	var out []state.ObjID
	for _, t := range o.Remembered {
		if !t.IsPlayer && t.Obj != 0 {
			out = append(out, t.Obj)
		}
	}
	return out
}

// TestCraftRecastKeepsItsCastMaterialAndStillCrafts: a craft card cast with
// a paid cast-time material, bounced to hand and recast with a second
// material, must still remember the material its NEW cast paid after the
// CR 400.7 battlefield entry clears its old incarnation's memory -- and
// must still be able to Craft afterwards. The events entry wipe splits the
// list at the boundary the push stamped (RememberedStackBase): entries the
// recast itself wrote survive, entries predating the push die with the
// entry.
func TestCraftRecastKeepsItsCastMaterialAndStillCrafts(t *testing.T) {
	t.Parallel()
	e := handEngine(t, card(t, recastCrafterScript))
	source := e.G.Zone(state.ZHand, 0)[0]
	castMat := graveyardCard(t, e, recastCastMaterialSrc)
	secondMat := graveyardCard(t, e, recastSecondMaterialSrc)

	// First cast: pay the mandatory graveyard-exile cost (castMat) and let
	// the creature resolve. A first battlefield entry (Incarnation 1) spares
	// the spell's remembered cast material outright.
	addMana(t, e, 0, "")
	castFirst(t, e, "cast")
	d := e.Pending()
	if d == nil || d.Kind != decision.KChoose {
		t.Fatalf("first cast's exile cost ask missing: %+v", d)
	}
	pick := -1
	for _, o := range d.Options {
		if o.Obj == castMat {
			pick = o.Index
		}
	}
	if pick < 0 {
		t.Fatalf("first cast's exile ask does not offer the cast material: %+v", d.Options)
	}
	submitChoices(t, e, pick)
	passUntilStackEmpty(t, e, 20)
	o := e.G.Obj(source)
	if o == nil || o.Zone != state.ZBattlefield {
		t.Fatalf("first cast: source = %+v, want it on the battlefield", o)
	}
	if got := rememberedObjsOf(o); len(got) != 1 || got[0] != castMat {
		t.Fatalf("precondition: after its first cast the crafter remembers %v, want exactly the paid material %d", got, castMat)
	}

	// Bounce it to its owner's hand. The leaving arm deliberately keeps the
	// remembered list: the old incarnation's pending referents must survive
	// the departure (the O-Ring / UntilHostLeavesPlay premise).
	e.emit(events.Event{Kind: events.MoveZone, Obj: source, From: state.ZBattlefield, To: state.ZHand})
	if o := e.G.Obj(source); o == nil || o.Zone != state.ZHand {
		t.Fatalf("bounce: source zone = %+v, want hand", o)
	}
	if got := rememberedObjsOf(e.G.Obj(source)); len(got) != 1 || got[0] != castMat {
		t.Fatalf("bounce: source Remembered = %v, want the prior stint's material kept for its pending referents", got)
	}

	// Recast it, paying a second material. The resolving spell now RE-enters
	// the battlefield (Incarnation 2), which is the entry the CR 400.7 wipe
	// serves: everything predating the push dies, the new cast's writes
	// survive.
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: e.G.Turn + 1})
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepMain1})
	e.pending = nil
	e.Advance()
	addMana(t, e, 0, "CC") // units for the later Craft activation
	castFirst(t, e, "cast")
	d2 := e.Pending()
	if d2 == nil || d2.Kind != decision.KChoose {
		t.Fatalf("second cast's exile cost ask missing: %+v", d2)
	}
	pick = -1
	for _, o := range d2.Options {
		if o.Obj == secondMat {
			pick = o.Index
		}
	}
	if pick < 0 {
		t.Fatalf("second cast's exile ask does not offer the second material: %+v", d2.Options)
	}
	submitChoices(t, e, pick)
	passUntilStackEmpty(t, e, 20)
	o = e.G.Obj(source)
	if o == nil || o.Zone != state.ZBattlefield || o.Incarnation < 2 {
		t.Fatalf("recast: source = %+v, want a battlefield re-entry (Incarnation 2)", o)
	}
	if got := rememberedObjsOf(o); len(got) != 1 || got[0] != secondMat {
		t.Fatalf("recast crafter Remembered = %v, want exactly the material its new cast paid (%d); the old stint's entry (%d) must die with the entry", got, secondMat, castMat)
	}

	// And it still Crafts: the activation exiles the artifact as its
	// material and returns the permanent transformed, with both the cast
	// material and the craft material on the transformed face (the
	// Transformed$ re-entry is the same object continuing, CR 712.4).
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: e.G.Turn + 1})
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepMain1})
	e.pending = nil
	e.Advance()
	craftMat := putGraveyard(t, e, 0, recastCraftMaterialSrc)
	addMana(t, e, 0, "CC")
	// The craft activation is the face's SECOND ability: the A:SP$ cast
	// line occupies index 0.
	option, ok := findAbilityOption(e, source, 1)
	if !ok {
		t.Fatalf("craft activation not offered for the re-entered crafter: %+v", e.Pending())
	}
	submitChoices(t, e, option.Index)
	dm := e.Pending()
	if dm == nil || dm.Kind != decision.KChoose || len(dm.Options) != 1 || dm.Options[0].Obj != craftMat {
		t.Fatalf("craft material ask = %+v, want exactly the graveyard artifact", dm)
	}
	submitChoices(t, e, dm.Options[0].Index)
	passUntilStackEmpty(t, e, 30)
	o = e.G.Obj(source)
	if o == nil || o.Zone != state.ZBattlefield || o.FaceIdx != 1 {
		t.Fatalf("craft return: source = %+v, want battlefield on the transformed face", o)
	}
	got := rememberedObjsOf(o)
	remembered := func(id state.ObjID) bool {
		for _, g := range got {
			if g == id {
				return true
			}
		}
		return false
	}
	if !remembered(secondMat) || !remembered(craftMat) {
		t.Fatalf("transformed source Remembered = %v, want the cast material %d and the craft material %d", got, secondMat, craftMat)
	}
}
