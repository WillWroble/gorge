package rules

// CR 506.5's attack-alone / block-alone restriction family, end to end:
// "CARDNAME can't attack alone." (Bonded Construct, Militia Rallier, Raging
// Kronch, Trusty Companion, Sightless Brawler's enchanted creature),
// "CARDNAME can't block alone." (Craven Hulk) and the compound "CARDNAME
// can't attack or block alone." (Mogg Flunkies, Ember Beast, Loyal Pegasus,
// Wojek Bodyguard, Bonded Horncrest, Jackal Familiar).
//
// The restriction binds the whole DECLARATION, never one creature, so every
// test below drives the REAL declare-attackers / declare-blockers decision
// (askAttackers / askBlockers -> Engine.Pending) and reads the three places
// the one rule must reach: the offered option fact (Option.AloneRestricted),
// the wire validator (decision.Decision.Validate) and the engine's own
// declaration check (validateAttackers / validateBlockers) -- plus the shared
// repair (botpolicy.Clamp -> decision.Decision.FitAlone) that keeps the
// deterministic bot's own answer legal.

import (
	"testing"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// aloneAttackersTable builds a two-seat table with the named corpus creatures
// on seat 0's battlefield (seat 0 active, in the declare-attackers step) and
// returns the decision the engine actually poses plus the offered option for
// each placed creature. Seat 1 is the only living opponent, so each creature
// has exactly one offered pair.
func aloneAttackersTable(t *testing.T, names ...string) (*Engine, *decision.Decision, map[string]int) {
	t.Helper()
	e := New(seatZeroStart(Config{Seed: 11, Names: []string{"a", "b"},
		Decks: [][]*cards.Card{mountainDeck(t, 40), mountainDeck(t, 40)}}))
	want := make(map[string]bool, len(names))
	byName := make(map[string]state.ObjID, len(names))
	for _, name := range names {
		c := corpusCard(t, name)
		id := onBoardCard(t, e, 0, c)
		e.G.Obj(id).SummonSick = false
		byName[c.Faces[0].Name] = id
		want[c.Faces[0].Name] = true
	}
	e.G.Active = 0
	e.G.Step = state.StepDeclareAttackers
	e.askAttackers()
	d := e.Pending()
	if d == nil || d.Kind != decision.KAttackers {
		t.Fatalf("no declare-attackers decision: %+v", d)
	}
	index := make(map[string]int, len(d.Options))
	for _, o := range d.Options {
		o := o
		name := e.G.Obj(o.Obj).Face().Name
		// Precondition: every named creature really is on the battlefield
		// with an offered pair, so the assertions below read a live option
		// rather than a zero value.
		if _, ok := index[name]; ok {
			t.Fatalf("precondition: %q has more than one offered pair", name)
		}
		index[name] = o.Index
	}
	for name := range want {
		if _, ok := index[name]; !ok {
			t.Fatalf("precondition: %q is not offered as an attacker (options: %v)", name, index)
		}
	}
	return e, d, index
}

// aloneBlockersTable parks the game in seat 0's declare-blockers step with
// the named corpus creatures as seat 0's would-be blockers and one plain
// attacker of seat 1's, and returns the posed decision plus the offered
// option index for each placed creature.
func aloneBlockersTable(t *testing.T, names ...string) (*Engine, *decision.Decision, map[string]int) {
	t.Helper()
	e := New(seatZeroStart(Config{Seed: 13, Names: []string{"a", "b"},
		Decks: [][]*cards.Card{mountainDeck(t, 40), mountainDeck(t, 40)}}))
	attacker := onBoard(t, e, 1, "Name:Rusher\nTypes:Creature\nPT:2/2\nOracle:x\n")
	want := make(map[string]bool, len(names))
	ids := make(map[string]state.ObjID, len(names))
	for _, name := range names {
		c := corpusCard(t, name)
		id := onBoardCard(t, e, 0, c)
		ids[c.Faces[0].Name] = id
		want[c.Faces[0].Name] = true
	}
	// The engine's own attack event shape: Player is the DEFENDER.
	attackSeat0(t, e, attacker)
	d := askBlockersFresh(t, e)
	if d == nil {
		t.Fatal("precondition: no declare-blockers decision was posed")
	}
	index := make(map[string]int, len(d.Options))
	for _, o := range d.Options {
		name := e.G.Obj(o.Obj).Face().Name
		if o.Attacker != attacker || !want[name] {
			continue
		}
		if _, ok := index[name]; ok {
			t.Fatalf("precondition: %q has more than one offered pair", name)
		}
		index[name] = o.Index
	}
	for name := range want {
		if _, ok := index[name]; !ok {
			t.Fatalf("precondition: %q is not offered as a blocker (placed %v)", name, ids)
		}
	}
	return e, d, index
}

// TestCantAttackOrBlockAlone pins the three sentences on both sides of the
// declaration: a lone declaration is refused (wire AND engine), a declaration
// beside a partner is accepted, and each sibling sentence binds only its own
// side. The last two cases are what keep the compound from being folded into
// the CantAttack/CantBlock per-pair restrictions, which would forbid the
// paired declaration too.
func TestCantAttackOrBlockAlone(t *testing.T) {
	t.Parallel()

	t.Run("lone attack rejected", func(t *testing.T) {
		t.Parallel()
		e, d, idx := aloneAttackersTable(t, "Mogg Flunkies", "Elite Vanguard")
		flunkies, partner := idx["Mogg Flunkies"], idx["Elite Vanguard"]
		// Precondition: the engine published the CR 506.5 fact on the
		// restricted creature's option and NOT on the plain one.
		if !d.Options[flunkies].AloneRestricted {
			t.Fatal("Mogg Flunkies' option is not marked AloneRestricted")
		}
		if d.Options[partner].AloneRestricted {
			t.Fatal("Elite Vanguard's option is marked AloneRestricted")
		}
		in := decision.Intent{Seq: d.Seq, Player: 0, Choices: []int{flunkies}}
		if err := d.Validate(in); err == nil {
			t.Fatal("Decision.Validate accepted Mogg Flunkies attacking alone")
		}
		if err := e.validateAttackers(d, in); err == nil {
			t.Fatal("validateAttackers accepted Mogg Flunkies attacking alone")
		}
	})

	t.Run("paired attack accepted", func(t *testing.T) {
		t.Parallel()
		e, d, idx := aloneAttackersTable(t, "Mogg Flunkies", "Elite Vanguard")
		in := decision.Intent{Seq: d.Seq, Player: 0, Choices: []int{idx["Mogg Flunkies"], idx["Elite Vanguard"]}}
		if err := d.Validate(in); err != nil {
			t.Fatalf("Decision.Validate rejected Mogg Flunkies beside a partner: %v", err)
		}
		if err := e.validateAttackers(d, in); err != nil {
			t.Fatalf("validateAttackers rejected Mogg Flunkies beside a partner: %v", err)
		}
	})

	t.Run("lone block rejected", func(t *testing.T) {
		t.Parallel()
		e, d, idx := aloneBlockersTable(t, "Mogg Flunkies", "Ember Beast")
		flunkies := idx["Mogg Flunkies"]
		if !d.Options[flunkies].AloneRestricted {
			t.Fatal("Mogg Flunkies' block option is not marked AloneRestricted")
		}
		in := decision.Intent{Seq: d.Seq, Player: 0, Choices: []int{flunkies}}
		if err := d.Validate(in); err == nil {
			t.Fatal("Decision.Validate accepted Mogg Flunkies blocking alone")
		}
		if err := e.validateBlockers(d, in); err == nil {
			t.Fatal("validateBlockers accepted Mogg Flunkies blocking alone")
		}
	})

	t.Run("paired block accepted", func(t *testing.T) {
		t.Parallel()
		e, d, idx := aloneBlockersTable(t, "Mogg Flunkies", "Ember Beast")
		in := decision.Intent{Seq: d.Seq, Player: 0, Choices: []int{idx["Mogg Flunkies"], idx["Ember Beast"]}}
		if err := d.Validate(in); err != nil {
			t.Fatalf("Decision.Validate rejected Mogg Flunkies beside a partner: %v", err)
		}
		if err := e.validateBlockers(d, in); err != nil {
			t.Fatalf("validateBlockers rejected Mogg Flunkies beside a partner: %v", err)
		}
	})

	t.Run("attack alone blocks alone", func(t *testing.T) {
		t.Parallel()
		// Bonded Construct prints only "can't attack alone.": the attack is
		// refused alone, while the SAME creature may block alone.
		_, d, idx := aloneAttackersTable(t, "Bonded Construct", "Elite Vanguard")
		in := decision.Intent{Seq: d.Seq, Player: 0, Choices: []int{idx["Bonded Construct"]}}
		if err := d.Validate(in); err == nil {
			t.Fatal("Decision.Validate accepted Bonded Construct attacking alone")
		}
		be, bd, bidx := aloneBlockersTable(t, "Bonded Construct")
		b := bidx["Bonded Construct"]
		if bd.Options[b].AloneRestricted {
			t.Fatal("Bonded Construct's block option is marked AloneRestricted (it can block alone)")
		}
		bin := decision.Intent{Seq: bd.Seq, Player: 0, Choices: []int{b}}
		if err := bd.Validate(bin); err != nil {
			t.Fatalf("Decision.Validate refused Bonded Construct blocking alone: %v", err)
		}
		if err := be.validateBlockers(bd, bin); err != nil {
			t.Fatalf("validateBlockers refused Bonded Construct blocking alone: %v", err)
		}
	})

	t.Run("block alone attacks alone", func(t *testing.T) {
		t.Parallel()
		// Craven Hulk prints only "can't block alone.": the lone block is
		// refused, while the same creature may attack alone.
		e, d, idx := aloneAttackersTable(t, "Craven Hulk")
		a := idx["Craven Hulk"]
		if d.Options[a].AloneRestricted {
			t.Fatal("Craven Hulk's attack option is marked AloneRestricted (it can attack alone)")
		}
		ain := decision.Intent{Seq: d.Seq, Player: 0, Choices: []int{a}}
		if err := d.Validate(ain); err != nil {
			t.Fatalf("Decision.Validate refused Craven Hulk attacking alone: %v", err)
		}
		if err := e.validateAttackers(d, ain); err != nil {
			t.Fatalf("validateAttackers refused Craven Hulk attacking alone: %v", err)
		}
		be, bd, bidx := aloneBlockersTable(t, "Craven Hulk")
		b := bidx["Craven Hulk"]
		if !bd.Options[b].AloneRestricted {
			t.Fatal("Craven Hulk's block option is not marked AloneRestricted")
		}
		bin := decision.Intent{Seq: bd.Seq, Player: 0, Choices: []int{b}}
		if err := bd.Validate(bin); err == nil {
			t.Fatal("Decision.Validate accepted Craven Hulk blocking alone")
		}
		if err := be.validateBlockers(bd, bin); err == nil {
			t.Fatal("validateBlockers accepted Craven Hulk blocking alone")
		}
	})

	t.Run("bot repair drops the lone declaration", func(t *testing.T) {
		t.Parallel()
		// The one-home rule: the shared repair must turn the bot's own lone
		// declaration into one the validator accepts, or the deterministic
		// bot re-submits the refused answer forever.
		_, d, idx := aloneAttackersTable(t, "Mogg Flunkies")
		lone := decision.Intent{Seq: d.Seq, Player: 0, Choices: []int{idx["Mogg Flunkies"]}}
		if err := d.Validate(lone); err == nil {
			t.Fatal("precondition: the unrepaired lone declaration was accepted")
		}
		repaired := botpolicy.Clamp(d, lone)
		if err := d.Validate(repaired); err != nil {
			t.Fatalf("Clamp/Validate refused the lone declaration: %v", err)
		}
		if len(repaired.Choices) != 0 {
			t.Fatalf("Clamp repaired the lone declaration to %v, want the empty declaration", repaired.Choices)
		}
	})

	t.Run("declaration commits when paired", func(t *testing.T) {
		t.Parallel()
		// The restriction must not leak into the per-pair attack oracle: the
		// paired declaration is really recorded as an attack.
		e, d, idx := aloneAttackersTable(t, "Mogg Flunkies", "Elite Vanguard")
		flunkies := d.Options[idx["Mogg Flunkies"]].Obj
		e.handleAttackers(d, decision.Intent{Seq: d.Seq, Player: 0,
			Choices: []int{idx["Mogg Flunkies"], idx["Elite Vanguard"]}})
		if o := e.G.Obj(flunkies); o == nil || !o.IsAttacking {
			t.Fatal("Mogg Flunkies did not attack beside its partner")
		}
		if got := e.G.Obj(flunkies).Attacking; got != 1 {
			t.Fatalf("Mogg Flunkies attacked seat %v, want the only opponent (seat 1)", got)
		}
	})
}
