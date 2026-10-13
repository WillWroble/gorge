package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// assertNoUnimplementedNotes fails on any loud unimplemented-Note this engine
// emits during the test: the implemented rider paths must be silent.
func assertNoUnimplementedNotes(t *testing.T, e *Engine) {
	t.Helper()
	for _, ev := range e.L.Events {
		if ev.Kind == events.Note && strings.Contains(strings.ToLower(ev.Text), "unimplemented") {
			t.Fatalf("loud unimplemented Note on the implemented rider path: %q", ev.Text)
		}
	}
}

// Test of Faith's prevention-shield rider (B4): "For each 1 damage prevented
// this way, put a +1/+1 counter on that creature" — the SP$ PreventDamage
// shield's PreventionSubAbility$ follow-up with the amount this application
// prevented bound as CounterNum$ PreventedDamage and the shield's own
// recipient bound through ShieldEffectTarget$ Targeted. The generic mechanism
// serves the whole rider family: PutCounter/Targeted (Test of Faith, Temper),
// GainLife/You (Candles' Glow) and the already-resolved DealDamage/
// ParentTarget (Acolyte's Reward, Vengeful Archon).

// TestTestOfFaithRiderPutsPreventedCounters resolves the REAL compiled Test
// of Faith spell ability: the shield installs on the targeted creature, one
// application of 3 prevented damage makes three +1/+1 counters and drains
// the pool, and no unimplemented Note marks the path.
func TestTestOfFaithRiderPutsPreventedCounters(t *testing.T) {
	t.Parallel()
	reg := sharedCorpus(t)

	t.Run("three prevented damage makes three counters", func(t *testing.T) {
		e := newSeats(t, 2)
		e.pending = nil
		faith := onBoardCard(t, e, 0, mustCorpusCard(t, reg, "Test of Faith"))
		lion := onBoard(t, e, 0, "Name:Lion\nTypes:Creature\nPT:2/2\nOracle:x\n")
		effects.Resolve(e, &effects.Ctx{Source: faith, Controller: 0,
			Targets:        []state.Target{{Obj: lion}},
			TargetsOffered: true}, e.G.Obj(faith).Face().SpellAbility())
		// Preconditions the assertion below depends on: the shield really
		// installed (the primitive ran, not a loud degenerate), it carries the
		// rider's recipient, and the lion starts with no counters.
		if got := countShieldCEs(e); got != 1 {
			t.Fatalf("shields = %d, want 1 installed on the lion", got)
		}
		if got := e.G.Obj(lion).Counter("P1P1"); got != 0 {
			t.Fatalf("counters before any damage = %d, want 0", got)
		}
		source := onBoard(t, e, 1, "Name:Bolter\nManaCost:R\nTypes:Creature\nPT:1/1\nOracle:x\n")
		e.damaging = source
		e.emit(events.Event{Kind: events.Damage, Obj: lion, Amount: 3})
		e.damaging = 0
		if got := e.G.Obj(lion).Damage; got != 0 {
			t.Fatalf("lion damage = %d, want 0: the shield prevented the 3", got)
		}
		if got := e.G.Obj(lion).Counter("P1P1"); got != 3 {
			t.Fatalf("+1/+1 counters = %d, want 3 from the 3 prevented damage", got)
		}
		if got := countShieldCEs(e); got != 0 {
			t.Fatalf("shields = %d, want 0: the 3-point pool is spent", got)
		}
		assertNoUnimplementedNotes(t, e)
		if got := countPreventedNotes(e, 3); got != 1 {
			t.Fatalf("prevention Notes of amount 3 = %d, want 1 (the shield applied)", got)
		}
	})

	t.Run("partial prevention counts part and keeps the pool", func(t *testing.T) {
		e := newSeats(t, 2)
		e.pending = nil
		faith := onBoardCard(t, e, 0, mustCorpusCard(t, reg, "Test of Faith"))
		lion := onBoard(t, e, 0, "Name:Lion\nTypes:Creature\nPT:2/2\nOracle:x\n")
		effects.Resolve(e, &effects.Ctx{Source: faith, Controller: 0,
			Targets:        []state.Target{{Obj: lion}},
			TargetsOffered: true}, e.G.Obj(faith).Face().SpellAbility())
		if got := countShieldCEs(e); got != 1 {
			t.Fatalf("shields = %d, want 1 installed on the lion", got)
		}
		source := onBoard(t, e, 1, "Name:Bolter\nManaCost:R\nTypes:Creature\nPT:1/1\nOracle:x\n")
		e.damaging = source
		e.emit(events.Event{Kind: events.Damage, Obj: lion, Amount: 2})
		if got := e.G.Obj(lion).Counter("P1P1"); got != 2 {
			t.Fatalf("+1/+1 counters = %d, want 2 from the 2 prevented of 3", got)
		}
		if got := countShieldCEs(e); got != 1 {
			t.Fatalf("shields = %d, want 1: one point of the 3-point pool stands", got)
		}
		e.emit(events.Event{Kind: events.Damage, Obj: lion, Amount: 1})
		e.damaging = 0
		if got := e.G.Obj(lion).Counter("P1P1"); got != 3 {
			t.Fatalf("+1/+1 counters = %d, want 3 across both applications", got)
		}
		if got := e.G.Obj(lion).Damage; got != 0 {
			t.Fatalf("lion damage = %d, want 0: all 3 damage was prevented", got)
		}
		if got := countShieldCEs(e); got != 0 {
			t.Fatalf("shields = %d, want 0: the pool is spent", got)
		}
		assertNoUnimplementedNotes(t, e)
	})
}

// TestCandlesGlowRiderGainsPreventedLife resolves the REAL compiled Candles'
// Glow: ShieldEffectTarget$ You binds the controller as the life rider's
// target, so the prevented amount is gained as life.
func TestCandlesGlowRiderGainsPreventedLife(t *testing.T) {
	t.Parallel()
	reg := sharedCorpus(t)
	e := newSeats(t, 2)
	e.pending = nil
	glow := onBoardCard(t, e, 0, mustCorpusCard(t, reg, "Candles' Glow"))
	bear := onBoard(t, e, 0, "Name:Bear\nTypes:Creature\nPT:2/2\nOracle:x\n")
	effects.Resolve(e, &effects.Ctx{Source: glow, Controller: 0,
		Targets:        []state.Target{{Obj: bear}},
		TargetsOffered: true}, e.G.Obj(glow).Face().SpellAbility())
	if got := countShieldCEs(e); got != 1 {
		t.Fatalf("shields = %d, want 1 installed on the bear", got)
	}
	source := onBoard(t, e, 1, "Name:Bolter\nManaCost:R\nTypes:Creature\nPT:1/1\nOracle:x\n")
	e.damaging = source
	e.emit(events.Event{Kind: events.Damage, Obj: bear, Amount: 2})
	e.damaging = 0
	if got := e.G.Obj(bear).Damage; got != 0 {
		t.Fatalf("bear damage = %d, want 0: the shield prevented the 2", got)
	}
	if got := e.G.Players[0].Life; got != 22 {
		t.Fatalf("controller life = %d, want 22: the 2 prevented damage is gained as life", got)
	}
	if got := countShieldCEs(e); got != 1 {
		t.Fatalf("shields = %d, want 1: one point of the 3-point pool stands", got)
	}
	e.emit(events.Event{Kind: events.Damage, Obj: bear, Amount: 1})
	if got := e.G.Players[0].Life; got != 23 {
		t.Fatalf("controller life = %d, want 23 after the last prevented point", got)
	}
	if got := countShieldCEs(e); got != 0 {
		t.Fatalf("shields = %d, want 0: the pool is spent", got)
	}
	assertNoUnimplementedNotes(t, e)
}

// TestPreventionRiderCoverageSyntheticMatchesCorpus pins the coverage gate
// against the REAL compiled corpus: the rider families this engine resolves
// (Test of Faith, Temper, Candles' Glow, Acolyte's Reward, Vengeful Archon)
// carry no api:PreventDamage.OtherRider, while the DB$ Effect riders this
// engine does not resolve (Sacred Boon, Scars of the Veteran) carry it and so
// are counted unsupported instead of playable-with-a-loud-Note.
func TestPreventionRiderCoverageSyntheticMatchesCorpus(t *testing.T) {
	t.Parallel()
	reg := sharedCorpus(t)
	resolved := []string{"Test of Faith", "Temper", "Candles' Glow",
		"Acolyte's Reward", "Vengeful Archon"}
	unresolved := []string{"Sacred Boon", "Scars of the Veteran"}
	hasRider := func(name string) bool {
		for _, p := range mustCorpusCard(t, reg, name).Primitives() {
			if p == "api:PreventDamage.OtherRider" {
				return true
			}
		}
		return false
	}
	for _, name := range resolved {
		if hasRider(name) {
			t.Fatalf("%s carries api:PreventDamage.OtherRider, want none: its rider is resolved", name)
		}
	}
	for _, name := range unresolved {
		if !hasRider(name) {
			t.Fatalf("%s lacks api:PreventDamage.OtherRider, want it: its DB$ Effect rider is unresolved", name)
		}
	}
}
