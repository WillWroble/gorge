package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// Pins for the "unexplained" rows of the 2026-10-10 corpus coverage census
// (archived 2026-10-10-corpus-coverage-census.md (docs/design/ARCHIVE.md)): Walking
// Ballista {4}, Mount Doom #2 and Mirrorpool #1 are payable-after-tapping
// (PotentialActions) exactly when their printed gates hold, so the census's
// "never potential" came from the bot's board, not from the offer walk.

func covPotentialAbility(e *Engine, obj state.ObjID, ability int) bool {
	for _, a := range e.PotentialActions(0) {
		if a.Kind == "ability" && a.Obj == obj && a.Ability == ability {
			return true
		}
	}
	return false
}

func covProbeLands(t *testing.T, e *Engine, n int, produced string) []state.ObjID {
	t.Helper()
	var ids []state.ObjID
	for i := 0; i < n; i++ {
		l := e.G.AddObject(card(t, "Name:Probe Land\nTypes:Land\nA:AB$ Mana | Cost$ T | Produced$ "+produced+"\nOracle:x\n"), 0)
		l.Zone = state.ZBattlefield
		ids = append(ids, l.ID)
	}
	return ids
}

// covPlaceProbe moves the first hand card to seat 0's battlefield beside extra.
func covPlaceProbe(t *testing.T, e *Engine, extra ...state.ObjID) state.ObjID {
	t.Helper()
	id := e.G.Zone(state.ZHand, 0)[0]
	e.G.Obj(id).Zone = state.ZBattlefield
	e.G.SetZone(state.ZHand, 0, nil)
	e.G.SetZone(state.ZBattlefield, 0, append([]state.ObjID{id}, extra...))
	e.priorityRound()
	if o := e.G.Obj(id); o.Zone != state.ZBattlefield {
		t.Fatalf("fixture: %s not on the battlefield", o.Face().Name)
	}
	return id
}

// Walking Ballista's "{4}: put a +1/+1 counter" ability#0 is potential with
// four untapped mana sources and not with three. The census saw 0 potential
// games because the bot machine-guns the counters off in the turn it casts
// the Ballista, with <= 3 untapped sources left (measured, 31 priority
// decisions over 580 games).
func TestWalkingBallistaFourManaAbilityPotentialAtFourSources(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		lands int
		want  bool
	}{{3, false}, {4, true}} {
		e := handEngine(t, corpusAlternativeCard(t, "Walking Ballista"))
		hand := e.G.Zone(state.ZHand, 0)[0]
		e.G.Obj(hand).AddCounter("P1P1", 2)
		id := covPlaceProbe(t, e, covProbeLands(t, e, tc.lands, "C")...)
		if got := covPotentialAbility(e, id, 0); got != tc.want {
			t.Fatalf("%d untapped sources: Ballista {4} potential = %v, want %v", tc.lands, got, tc.want)
		}
		if tc.want && !covPotentialAbility(e, id, 1) {
			t.Fatalf("remove-counter ability#1 not potential: precondition (counters on the Ballista) failed")
		}
	}
}

// Mount Doom's third ability needs a legendary artifact to sacrifice. The
// only repo deck carrying Mount Doom (rakdos-muscle-scam-exe) has no
// legendary artifact, so its ability#2 can never be payable there.
func TestMountDoomSacrificeAbilityNeedsALegendaryArtifact(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"Name:Probe Relic\nTypes:Legendary Artifact\nOracle:x\n", true},
		{"Name:Probe Relic\nTypes:Artifact\nOracle:x\n", false},
	} {
		e := handEngine(t, corpusAlternativeCard(t, "Mount Doom"))
		relic := e.G.AddObject(card(t, tc.src), 0)
		relic.Zone = state.ZBattlefield
		extra := append([]state.ObjID{relic.ID}, covProbeLands(t, e, 4, "B")...)
		extra = append(extra, covProbeLands(t, e, 4, "R")...)
		id := covPlaceProbe(t, e, extra...)
		if !covPotentialAbility(e, id, 1) {
			t.Fatalf("ability#1 not potential: the board does not afford Mount Doom's own abilities")
		}
		if got := covPotentialAbility(e, id, 2); got != tc.want {
			t.Fatalf("%q: Mount Doom #2 potential = %v, want %v", tc.src[:30], got, tc.want)
		}
	}
}

// Mirrorpool's copy-spell ability#1 targets "Instant.YouCtrl,
// Sorcery.YouCtrl" with TargetType$ Spell: it is potential only while an
// instant or sorcery of its controller is on the stack.
func TestMirrorpoolCopySpellNeedsYourSpellOnTheStack(t *testing.T) {
	t.Parallel()
	bolt := "Name:Probe Bolt\nManaCost:R\nTypes:Instant\nA:SP$ DealDamage | ValidTgts$ Any | NumDmg$ 3 | SpellDescription$ x\nOracle:x\n"
	e := handEngine(t, corpusAlternativeCard(t, "Mirrorpool"), card(t, bolt))
	hand := e.G.Zone(state.ZHand, 0)
	id, boltID := hand[0], hand[1]
	e.G.Obj(id).Zone = state.ZBattlefield
	e.G.SetZone(state.ZHand, 0, []state.ObjID{boltID})
	ls := append(covProbeLands(t, e, 1, "R"), covProbeLands(t, e, 3, "C")...)
	e.G.SetZone(state.ZBattlefield, 0, append([]state.ObjID{id}, ls...))
	addMana(t, e, 0, "R")
	if covPotentialAbility(e, id, 1) {
		t.Fatalf("copy-spell potential with an empty stack")
	}
	cast := -1
	for _, o := range e.Pending().Options {
		if o.Kind == "cast" && o.Obj == boltID {
			cast = o.Index
		}
	}
	if cast < 0 {
		t.Fatalf("Probe Bolt not castable from the floating R: %+v", e.Pending().Options)
	}
	submitChoices(t, e, cast)
	for i := 0; i < 6 && e.Pending() != nil && e.Pending().Kind != "priority"; i++ {
		submitChoices(t, e, 0)
	}
	if len(e.G.Stack) != 1 {
		t.Fatalf("stack = %d, want the bolt on it", len(e.G.Stack))
	}
	if !covPotentialAbility(e, id, 1) {
		t.Fatalf("copy-spell ability#1 not potential with Probe Bolt on the stack and 3 sources (one {C}-producing x3) untapped")
	}
}
