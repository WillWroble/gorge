package rules

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// A combat DamageDone-family trigger binds AttackingPlayer (the attacker's
// controller) beside DefendingPlayer, so Defined$/NewController$
// TriggeredAttackingPlayer resolves on Contested Game Ball and Norn's Decree.
func TestDamageDoneTriggerBindsAttackingPlayer(t *testing.T) {
	t.Parallel()
	e := layerEngine(t)
	source := e.G.Zone(state.ZLibrary, 0)[0]
	atk := e.G.Zone(state.ZLibrary, 1)[0]
	o := e.G.Obj(atk)
	o.Controller, o.IsAttacking, o.Attacking = 1, true, 0
	if !o.IsAttacking || o.Controller != 1 {
		t.Fatal("precondition: attacker setup")
	}
	e.damaging = atk
	// Every damage-family mode the arm covers, by name: DamageDone,
	// DamageDoneOnce and DamageDealtOnce (single) and the batch modes
	// DamageAll and ExcessDamageAll (Kediss-style DamageSource$ overrides)
	// route through the same case arm, so all five bind the role.
	for _, mode := range []string{"DamageDone", "DamageDoneOnce", "DamageDealtOnce", "DamageAll", "ExcessDamageAll"} {
		c := e.triggerReferents(cards.Trigger{Mode: mode}, source, events.Event{Kind: events.Damage, Player: 0, Amount: 2}, nil)
		if want := (state.Target{IsPlayer: true, Player: 1}); c.AttackingPlayer != want {
			t.Errorf("%s: AttackingPlayer = %+v, want %+v", mode, c.AttackingPlayer, want)
		}
		if want := (state.Target{IsPlayer: true, Player: 0}); c.DefendingPlayer != want {
			t.Errorf("%s: DefendingPlayer = %+v, want %+v", mode, c.DefendingPlayer, want)
		}
	}
	// A non-attacking source binds neither role.
	o.IsAttacking = false
	c := e.triggerReferents(cards.Trigger{Mode: "DamageDone"}, source, events.Event{Kind: events.Damage, Player: 0, Amount: 2}, nil)
	if c.AttackingPlayer.IsPlayer || c.DefendingPlayer.IsPlayer {
		t.Errorf("non-attacking source bound combat roles: %+v", c)
	}
}

// Contested Game Ball on the real corpus script: seat 0's Hill Giant connects
// with seat 1, and seat 0 (the attacking player) gains control of the Ball.
func TestContestedGameBallPassesToAttackingPlayer(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	e, cfg := combatTriggerBoard(t, reg, []string{"Hill Giant"}, nil, []string{"Contested Game Ball"}, nil)
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: 2})
	e.emit(events.Event{Kind: events.StepChange, Step: state.StepDeclareAttackers})
	ball := findBattlefield(t, e, 1, "Contested Game Ball", 0)
	giant := findBattlefield(t, e, 0, "Hill Giant", 0)
	if e.G.Obj(ball).Controller != 1 {
		t.Fatal("precondition: seat 1 controls the Ball")
	}
	e.askAttackers()
	submitAttackers(t, e, giant)
	passUntilStackEmpty(t, e, 30)
	if e.G.Players[1].Life >= 20 {
		t.Fatal("precondition: combat damage was not dealt")
	}
	if got := e.G.Obj(ball).Controller; got != 0 {
		t.Fatalf("Ball controller = %d, want 0 (the attacking player)", got)
	}
	replayCheck(t, e, cfg)
}

// Census: the corpus faces whose damage-family trigger body reads
// TriggeredAttackingPlayer. Each depends on the DamageDone arm binding the
// role; a new card joining this set must be added with a card test.
func TestDamageTriggerAttackingPlayerCensus(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	var got []string
	for _, c := range reg.AllCards() {
		for _, f := range c.Faces {
			reads := false
			for _, body := range f.SVars {
				if strings.Contains(body, "TriggeredAttackingPlayer") {
					reads = true
				}
			}
			if !reads {
				continue
			}
			seen := map[cards.TriggerMode]bool{}
			for _, tr := range f.Triggers {
				switch tr.ModeKind() {
				case cards.TriggerDamageDone, cards.TriggerDamageDealtOnce, cards.TriggerDamageDoneOnce, cards.TriggerDamageAll, cards.TriggerExcessDamageAll:
					if !seen[tr.ModeKind()] {
						seen[tr.ModeKind()] = true
						got = append(got, f.Name)
					}
				}
			}
		}
	}
	sort.Strings(got)
	want := []string{"Contested Game Ball", "Norn's Decree"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("damage triggers reading TriggeredAttackingPlayer = %v, want %v", got, want)
	}
}
