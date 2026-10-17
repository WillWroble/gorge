package rules

// Producing-source mana provenance for a cast (engine/rules/cast_mana_sources.go):
// the SpellCast trigger's `ValidSA$ Spell.ManaFromCard.StrictlySelf`
// ("whenever you cast a permanent spell using mana produced by CARDNAME",
// Tecutlan, the Searing Rift; The Myriad Pools; Barracks of the Thousand).
// CR 106.3 and CR 601.2h: mana is produced by a source and spent while
// paying the spell's total cost, so the trigger asks which source produced
// the units this cast's payment spent.
//
// The carrier below keeps Tecutlan's printed {T}: Add {R} and its SpellCast
// trigger line verbatim except for the Execute$ body, which is a synthetic
// GainLife so the fire is observable without a discover library walk
// (fixtures are inline per the licensing rule).

import (
	"slices"
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

const tecutlanManaSrc = "Name:Tecutlan Test\nManaCost:2 R\nTypes:Legendary Artifact\n" +
	"A:AB$ Mana | Cost$ T | Produced$ R | SpellDescription$ Add {R}.\n" +
	"T:Mode$ SpellCast | ValidCard$ Permanent | ValidSA$ Spell.ManaFromCard.StrictlySelf | ValidActivatingPlayer$ You | Execute$ TrigGain | TriggerDescription$ Whenever you cast a permanent spell using mana produced by CARDNAME, you gain 1 life.\n" +
	"SVar:TrigGain:DB$ GainLife | Defined$ You | LifeAmount$ 1\n" +
	"Oracle:x\n"

const (
	mfcGoblinSrc    = "Name:MFC Goblin\nManaCost:R\nTypes:Creature Goblin\nPT:1/1\nOracle:x\n"
	mfcGoblinTwoSrc = "Name:MFC Goblin Two\nManaCost:R\nTypes:Creature Goblin\nPT:1/1\nOracle:x\n"
	mfcOgreSrc      = "Name:MFC Ogre\nManaCost:1 R\nTypes:Creature Ogre\nPT:2/2\nOracle:x\n"
	mfcInstantSrc   = "Name:MFC Ritual\nManaCost:R\nTypes:Instant\nA:SP$ GainLife | Defined$ You | LifeAmount$ 5\nOracle:x\n"
)

// mfcMountain moves a library Mountain onto seat 0's battlefield (a logged
// MoveZone) and returns it.
func mfcMountain(t *testing.T, e *Engine) state.ObjID {
	t.Helper()
	for _, id := range e.G.Zone(state.ZLibrary, 0) {
		if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Mountain" {
			e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZLibrary, To: state.ZBattlefield})
			e.pending = nil
			return id
		}
	}
	t.Fatal("no Mountain in seat 0's library")
	return 0
}

func TestManaFromCardStrictlySelfReadsTheSpentSource(t *testing.T) {
	t.Parallel()
	e, cfg, _ := etbConfig(t, 53, []string{tecutlanManaSrc, mfcGoblinSrc, mfcGoblinTwoSrc, mfcOgreSrc, mfcInstantSrc}, nil)
	tec := moveSeeded(t, e, 0, tecutlanManaSrc, state.ZBattlefield)
	goblin := findByName(e, "MFC Goblin", 0)
	goblinTwo := findByName(e, "MFC Goblin Two", 0)
	ogre := findByName(e, "MFC Ogre", 0)
	ritual := findByName(e, "MFC Ritual", 0)
	mountain := mfcMountain(t, e)
	life := func() int32 { return e.G.Players[0].Life }
	start := life()

	// 1. Tecutlan's own mana pays for a permanent spell: the pool holds it
	// as a source-bearing batch, the cast records the source, the trigger
	// fires.
	e.priorityRound()
	activateMana(t, e, tec)
	if n := len(e.G.Players[0].RestrictedMana); n != 1 || e.G.Players[0].RestrictedMana[0].Source != tec {
		t.Fatalf("pool batches %+v, want one batch whose Source is Tecutlan %d", e.G.Players[0].RestrictedMana, tec)
	}
	castFirstOf(t, e, goblin)
	if got := e.G.Obj(goblin).ManaSpentSources; !slices.Equal(got, []state.ObjID{tec}) {
		t.Fatalf("goblin ManaSpentSources = %v, want [%d]", got, tec)
	}
	if got := pushCount(e, tec); got != 1 {
		t.Fatalf("TriggerPush after the Tecutlan-paid cast = %d, want 1", got)
	}
	drainTriggerAsks(t, e, 30)
	if got := life(); got != start+1 {
		t.Fatalf("life after the trigger = %d, want %d", got, start+1)
	}

	// 2. Control: a land's mana pays for a permanent spell. No source is
	// observed, so no CastInfo and no fire.
	e.priorityRound()
	activateMana(t, e, mountain)
	if n := len(e.G.Players[0].RestrictedMana); n != 0 {
		t.Fatalf("a Mountain's mana made %d pool batches, want 0 (plain mana stays plain)", n)
	}
	castFirstOf(t, e, goblinTwo)
	if got := e.G.Obj(goblinTwo).ManaSpentSources; len(got) != 0 {
		t.Fatalf("land-paid cast ManaSpentSources = %v, want none", got)
	}
	drainTriggerAsks(t, e, 30)
	if got := pushCount(e, tec); got != 1 {
		t.Fatalf("TriggerPush after the land-paid cast = %d, want still 1", got)
	}

	// 3. Tecutlan's mana on a NONpermanent spell: the source is recorded,
	// but ValidCard$ Permanent keeps the trigger silent.
	e.emit(events.Event{Kind: events.Untap, Obj: tec})
	e.emit(events.Event{Kind: events.Untap, Obj: mountain})
	e.pending = nil
	e.priorityRound()
	activateMana(t, e, tec)
	castFirstOf(t, e, ritual)
	if got := e.G.Obj(ritual).ManaSpentSources; !slices.Equal(got, []state.ObjID{tec}) {
		t.Fatalf("ritual ManaSpentSources = %v, want [%d]", got, tec)
	}
	drainTriggerAsks(t, e, 30)
	if got := pushCount(e, tec); got != 1 {
		t.Fatalf("TriggerPush after a Tecutlan-paid instant = %d, want still 1", got)
	}

	// 4. Mixed pool: Tecutlan's R and the Mountain's R both pay {1}{R}; the
	// spend includes Tecutlan's unit, so the trigger fires.
	e.emit(events.Event{Kind: events.Untap, Obj: tec})
	e.pending = nil
	e.priorityRound()
	activateMana(t, e, tec)
	e.priorityRound()
	activateMana(t, e, mountain)
	castFirstOf(t, e, ogre)
	if got := e.G.Obj(ogre).ManaSpentSources; !slices.Equal(got, []state.ObjID{tec}) {
		t.Fatalf("ogre ManaSpentSources = %v, want [%d]", got, tec)
	}
	drainTriggerAsks(t, e, 30)
	if got := pushCount(e, tec); got != 2 {
		t.Fatalf("TriggerPush after the mixed-pool cast = %d, want 2", got)
	}
	replayCheck(t, e, cfg)
}
