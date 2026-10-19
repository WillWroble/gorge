package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// CR 510.4 / 117.4: a dies trigger put on the stack in the priority round
// between the first-strike and regular combat damage steps must resolve
// before the step ends; the step only ends when all players pass in
// succession with an EMPTY stack.
func TestFirstStrikeDiesTriggerResolvesBeforeRegularDamage(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	e, cfg := combatTriggerBoard(t, reg, nil,
		[]string{"Name:Striker\nManaCost:1 W\nTypes:Creature Human\nPT:3/3\nK:First Strike\nOracle:x\n"},
		[]string{"Perilous Myr"}, nil)
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: 2})
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepDeclareAttackers})
	striker := findBattlefield(t, e, 0, "Striker", 0)
	myr := findBattlefield(t, e, 1, "Perilous Myr", 0)

	e.askAttackers()
	submitAttackersOnly(t, e, striker)
	for i := 0; i < 64; i++ {
		d := e.Pending()
		if d == nil {
			t.Fatal("no pending decision")
		}
		if d.Kind == decision.KBlockers {
			submitBlockersOnly(t, e, myr)
			break
		}
		submitPass(t, e)
	}
	sawTrigger, nonActiveHadPriority := false, false
	for i := 0; i < 64; i++ {
		d := e.Pending()
		if d == nil {
			t.Fatal("no pending decision")
		}
		t.Logf("decision %s p%d step=%v stack=%d passes=%d prio=%d", d.Kind, d.Player, e.G.Step, len(e.G.Stack), e.G.Passes, e.G.Priority)
		if len(e.G.Stack) > 0 {
			sawTrigger = true
			if d.Kind == decision.KPriority && d.Player == 1 {
				nonActiveHadPriority = true
			}
		}
		if e.G.Step > state.StepCombatDamage {
			// CR 117.3a: the end of combat step opens with the active
			// player holding priority.
			if d.Kind != decision.KPriority || d.Player != e.G.Active || e.G.Passes != 0 {
				t.Fatalf("end of combat opened with %s for seat %d (passes %d), want priority for active seat %d with no passes",
					d.Kind, d.Player, e.G.Passes, e.G.Active)
			}
			break
		}
		switch d.Kind {
		case decision.KPriority:
			submitPass(t, e)
		default:
			// The Myr's "any target" pick: the first offered option.
			if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{0}}); err != nil {
				t.Fatalf("submit %s: %v", d.Kind, err)
			}
		}
		if e.G.Step > state.StepCombatDamage && len(e.G.Stack) > 0 {
			t.Fatalf("combat damage step ended with %d object(s) on the stack (CR 117.4)", len(e.G.Stack))
		}
	}
	if !sawTrigger {
		t.Fatal("Perilous Myr's dies trigger never reached the stack")
	}
	if !nonActiveHadPriority {
		t.Fatal("the non-active player never received priority while the trigger was on the stack (CR 117.3b)")
	}
	if o := e.G.Obj(myr); o.Zone != state.ZGraveyard {
		t.Fatalf("myr zone %v, want graveyard", o.Zone)
	}
	replayCheck(t, e, cfg)
}

// CR 508.8 + 117.3a: a combat with no attackers skips to the end of combat
// step after the declare-attackers priority round, and that step still
// opens with the ACTIVE player holding priority, no pass counted.
func TestNoAttackersEndOfCombatOpensWithActivePriority(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	e, _ := combatTriggerBoard(t, reg, nil,
		[]string{"Name:Striker\nManaCost:1 W\nTypes:Creature Human\nPT:3/3\nOracle:x\n"}, nil, nil)
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: 2})
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepDeclareAttackers})
	e.askAttackers()
	d := e.Pending()
	if d == nil || d.Kind != decision.KAttackers {
		t.Fatalf("expected an attackers decision, got %+v", d)
	}
	if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player}); err != nil {
		t.Fatalf("declare no attackers: %v", err)
	}
	for i := 0; i < 8 && e.G.Step == state.StepDeclareAttackers; i++ {
		submitPass(t, e)
	}
	if e.G.Step != state.StepEndCombat {
		t.Fatalf("step %v, want end of combat", e.G.Step)
	}
	d = e.Pending()
	if d == nil || d.Kind != decision.KPriority || d.Player != 0 || e.G.Passes != 0 {
		t.Fatalf("end of combat opened with %+v (passes %d), want priority for active seat 0 with no passes", d, e.G.Passes)
	}
}

// CR 510.4: any action in the between-passes priority round -- here the
// active player tapping a land for mana -- keeps the round open. It used to
// re-enter combatStep, which ran the regular damage pass and moved to end of
// combat, emptying the mana the player had just made (CR 500.4): a payment
// walked through that window (kfuzz gorge-vs-gorge seed 1, an instant cast
// in the first-strike window) stopped halfway with an empty pool.
func TestFirstStrikeWindowManaAbilityKeepsTheStep(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	e, cfg := combatTriggerBoard(t, reg, []string{"Plains"},
		[]string{"Name:Striker\nManaCost:1 W\nTypes:Creature Human\nPT:3/3\nK:First Strike\nOracle:x\n"},
		nil, nil)
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: 2})
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepDeclareAttackers})
	striker := findBattlefield(t, e, 0, "Striker", 0)
	plains := findBattlefield(t, e, 0, "Plains", 0)
	e.askAttackers()
	submitAttackersOnly(t, e, striker)
	for i := 0; i < 16 && e.G.Step != state.StepCombatDamage; i++ {
		d := e.Pending()
		if d.Kind == decision.KBlockers {
			if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player}); err != nil {
				t.Fatalf("no blocks: %v", err)
			}
			continue
		}
		submitPass(t, e)
	}
	d := e.Pending()
	if e.G.Step != state.StepCombatDamage || d == nil || d.Kind != decision.KPriority || d.Player != 0 {
		t.Fatalf("not at the active player's between-passes priority: step %v, %+v", e.G.Step, d)
	}
	if got := e.G.Players[1].Life; got != 17 {
		t.Fatalf("defender life %d after first-strike damage, want 17", got)
	}
	idx := -1
	for _, o := range d.Options {
		if o.Kind == "activate" && o.Obj == plains {
			idx = o.Index
		}
	}
	if idx < 0 {
		t.Fatalf("Plains mana ability not offered: %+v", d.Options)
	}
	if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{idx}}); err != nil {
		t.Fatalf("activate Plains: %v", err)
	}
	if e.G.Step != state.StepCombatDamage {
		t.Fatalf("a mana ability in the between-passes round moved the game to %v", e.G.Step)
	}
	if d := e.Pending(); d == nil || d.Kind != decision.KPriority || d.Player != 0 {
		t.Fatalf("after the mana ability: %+v, want the active player's priority", d)
	}
	if w := e.G.Players[0].Pool[state.MW]; w != 1 {
		t.Fatalf("pool W = %d after tapping Plains, want 1", w)
	}
	drainCombatDamagePriority(t, e)
	if e.G.Step != state.StepEndCombat {
		t.Fatalf("step %v after the round, want end of combat", e.G.Step)
	}
	replayCheck(t, e, cfg)
}
