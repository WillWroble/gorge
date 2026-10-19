package rules

import (
	"strconv"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// dredgeAskInDrawStep drives seat 0's second turn (turn 3, so the turn-based
// draw is not skipped) up to the draw step and reports the DREDGE ask the draw
// poses, or nil when the draw went through as an ordinary draw. Since is the
// event index seat 0's turn 3 upkeep starts at, so a caller can inspect every
// event of the turn whose draw step is checked.
//
// It passes priority itself and declines combat: a board that keeps a creature
// alive (the deployed Jailer) reaches a declare-attackers ask that driveToStep
// stops on, so the drive cannot be driveToStep alone.
func dredgeAskInDrawStep(t *testing.T, e *Engine) (d *decision.Decision, since int) {
	t.Helper()
	// Every step crossed on the way, answered the way driveToStep answers a
	// priority: pass. A deployed creature (the Jailer) reaches declare-attackers,
	// which earlier steps did not, so it is declined here rather than fatal.
	steps := []struct {
		turn   int32
		active state.PlayerID
		step   state.Step
	}{
		{3, 0, state.StepUpkeep},
		{3, 0, state.StepDraw},
	}
	since = -1
	for _, target := range steps {
		if target.step == state.StepUpkeep {
			since = len(e.L.Events)
		}
		for i := 0; i < 4000; i++ {
			if e.G.Turn == target.turn && e.G.Active == target.active && e.G.Step == target.step {
				break
			}
			if e.G.Over {
				t.Fatalf("game ended before turn %d seat %d step %s", target.turn, target.active, target.step)
			}
			if answerIfDiscard(t, e) {
				continue
			}
			d := e.Pending()
			if d == nil || d.Kind != decision.KPriority && d.Kind != decision.KAttackers {
				t.Fatalf("non-priority decision %+v encountered while driving to turn %d seat %d step %s",
					d, target.turn, target.active, target.step)
			}
			if d.Kind == decision.KAttackers {
				if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{}}); err != nil {
					t.Fatalf("submit empty attackers: %v", err)
				}
				continue
			}
			idx := -1
			for _, o := range d.Options {
				if o.Kind == "pass" {
					idx = o.Index
				}
			}
			if idx < 0 {
				t.Fatalf("priority decision with no pass option: %+v", d)
			}
			if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{idx}}); err != nil {
				t.Fatalf("submit: %v", err)
			}
		}
	}
	pending := e.Pending()
	if pending != nil && pending.Kind == decision.KModes && pending.ResumeKind == "dredge" {
		return pending, since
	}
	return nil, since
}

// TestDredgeAbilityLoss is the CR 613.1f / 702.52a pin: a card in a graveyard
// under "cards in graveyards lose all abilities" (Yixlid Jailer's
// RemoveAllAbilities$ static, AffectedZone$ Graveyard) has no dredge, so the
// turn draw offers no dredge replacement and draws normally. The positive
// control in the same test runs the identical board WITHOUT the Jailer: the
// same Brownscale must then offer Dredge 2, which is what proves the negative
// is the Jailer's ability loss and not a setup that never had a dredger.
func TestDredgeAbilityLoss(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	brownscale, ok := reg.Lookup("Golgari Brownscale")
	if !ok {
		t.Fatal("Golgari Brownscale missing from corpus")
	}
	jailer, ok := reg.Lookup("Yixlid Jailer")
	if !ok {
		t.Fatal("Yixlid Jailer missing from corpus")
	}
	if _, ok := brownscale.Faces[0].KeywordParam("Dredge"); !ok {
		t.Fatal("precondition: Golgari Brownscale prints no Dredge keyword")
	}
	// The Jailer must be an ability remover, or the scenario proves nothing.
	remover := false
	for i := range jailer.Faces[0].Statics {
		if jailer.Faces[0].Statics[i].HasParam(cards.PKRemoveAllAbilities) {
			remover = true
		}
	}
	if !remover {
		t.Fatal("precondition: Yixlid Jailer carries no RemoveAllAbilities$ static")
	}

	// run deals the Brownscale into seat 0's graveyard (and, when withJailer,
	// deploys seat 1's Jailer in time for seat 0's draw step) and returns the
	// engine plus the Brownscale's id.
	run := func(t *testing.T, withJailer bool) (*Engine, state.ObjID) {
		t.Helper()
		deck := append([]*cards.Card{brownscale}, mountainDeck(t, 39)...)
		cfg := seatZeroStart(Config{Seed: 245, Names: []string{"a", "b"},
			Decks: [][]*cards.Card{deck, mountainDeck(t, 40)}, Tokens: map[string]*cards.Card{}})
		e := New(cfg)
		e.Advance()
		var bid state.ObjID
		for _, z := range []state.Zone{state.ZHand, state.ZLibrary} {
			for _, id := range e.G.Zone(z, 0) {
				if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Golgari Brownscale" {
					bid = id
				}
			}
		}
		if bid == 0 {
			t.Fatal("Golgari Brownscale was not dealt to seat 0")
		}
		e.emit(events.Event{Kind: events.MoveZone, Obj: bid, From: e.G.Obj(bid).Zone, To: state.ZGraveyard})
		if o := e.G.Obj(bid); o == nil || o.Zone != state.ZGraveyard {
			t.Fatalf("precondition: Brownscale not in the graveyard: %+v", o)
		}
		if !withJailer {
			return e, bid
		}
		// Seat 1's Jailer: created in its hand, then moved to its battlefield
		// by the same event path a cast uses, so the static scan sees it.
		o := e.G.AddObject(jailer, 1)
		e.emit(events.Event{Kind: events.MoveZone, Obj: o.ID, From: state.ZHand, To: state.ZBattlefield})
		if got := e.G.Obj(o.ID); got == nil || got.Zone != state.ZBattlefield {
			t.Fatalf("precondition: Jailer not on the battlefield: %+v", got)
		}
		// The measurement the fix rests on, asserted as a precondition: the
		// Jailer's static is a LIVE layer-6 removal scoped to the graveyard,
		// and it reaches a graveyard card -- the Brownscale's derived keyword
		// list is empty. Without this, a green negative below would only prove
		// the board has no dredger at all.
		jid := oidOnBattlefield(t, e, 1, "Yixlid Jailer")
		removal := false
		for _, ce := range e.active() {
			if ce.Source == jid && ce.Layer == state.LAbilities && ce.RemoveAbilities && ce.AffectedZone == "Graveyard" {
				removal = true
			}
		}
		if !removal {
			t.Fatal("precondition: no graveyard-scoped RemoveAbilities effect from Yixlid Jailer is live")
		}
		if kw := e.Derived(bid).Keywords; len(kw) != 0 {
			t.Fatalf("precondition: layer walk derives keywords %v for a graveyard card under Yixlid Jailer, want none", kw)
		}
		return e, bid
	}

	// Negative: under the Jailer, no dredge is offered and the ordinary draw
	// happens.
	t.Run("with-jailer", func(t *testing.T) {
		e, bid := run(t, true)
		d, since := dredgeAskInDrawStep(t, e)
		if d != nil {
			t.Fatalf("turn draw offered a dredge replacement for a card with no abilities: %s", dredgeOptionKinds(d))
		}
		drew := false
		for _, ev := range e.L.Events[since:] {
			if ev.Kind == events.Note && strings.Contains(ev.Text, "unimplemented") {
				t.Fatalf("draw handler did not complete normally: %+v", ev)
			}
			if ev.Kind == events.Draw && ev.Player == 0 {
				drew = true
			}
			if ev.Kind == events.MoveZone && ev.Obj == bid && ev.To == state.ZHand {
				t.Fatalf("the Brownscale left the graveyard on an ordinary draw: %+v", ev)
			}
		}
		if !drew {
			t.Fatal("no ordinary draw happened in the draw step")
		}
		if got := e.G.Obj(bid); got == nil || got.Zone != state.ZGraveyard {
			t.Fatalf("Brownscale left the graveyard: %+v", got)
		}
	})

	// Positive control: the same board without the Jailer offers Dredge 2.
	t.Run("without-jailer", func(t *testing.T) {
		e, bid := run(t, false)
		d, _ := dredgeAskInDrawStep(t, e)
		if d == nil {
			t.Fatal("control: the turn draw offered no dredge replacement without the Jailer")
		}
		found := false
		for _, opt := range d.Options {
			if opt.Kind == "dredge" && opt.Obj == bid && strings.Contains(opt.Label, "Dredge 2") {
				found = true
			}
		}
		if !found {
			t.Fatalf("control: no Dredge 2 option for the Brownscale: %s", dredgeOptionKinds(d))
		}
	})
}

// dredgeOptionKinds renders a decision's options compactly for a failure message: a
// full %+v dump of a decision is thousands of characters, which buries the
// assertion that failed.
func dredgeOptionKinds(d *decision.Decision) string {
	var b strings.Builder
	for _, o := range d.Options {
		b.WriteString(" [" + strconv.Itoa(o.Index) + "] " + o.Kind + " obj=" + strconv.FormatUint(uint64(o.Obj), 10) + " " + o.Label + ";")
	}
	return b.String()
}

// oidOnBattlefield returns the id of the named card on seat p's battlefield.
func oidOnBattlefield(t *testing.T, e *Engine, p state.PlayerID, name string) state.ObjID {
	t.Helper()
	for _, id := range e.G.Zone(state.ZBattlefield, p) {
		if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == name {
			return id
		}
	}
	t.Fatalf("precondition: %s is not on seat %d's battlefield (%v)", name, p, e.G.Zone(state.ZBattlefield, p))
	return 0
}
