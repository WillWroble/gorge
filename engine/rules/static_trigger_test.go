package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// The Static$ True trigger class: Forge's Trigger.isStatic() runs a trigger
// with that parameter immediately, without a stack object (it is a static
// ability, not a triggered ability, so CR 603.3b's order choice never sees
// it). The filing card is Oblivious Bookworm: its TurnFaceUp line imprints
// the turned permanent (the end-step Draw reads ImprintedSize) and its
// upkeep line clears that imprint. Neither line may be pushed.

// TestStaticTriggerTurnFaceUpResolvesOffStack pins the filing card's
// TurnFaceUp line: emitting the event imprints the turned permanent and
// never emits a TriggerPush for the Bookworm.
func TestStaticTriggerTurnFaceUpResolvesOffStack(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	bookworm := mustCorpusCard(t, reg, "Oblivious Bookworm")
	bearCard := card(t, "Name:Test Bear\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")
	e, cfg := tokenReplGame(t, 401, bookworm, bearCard)
	bookwormID := moveSeededCard(t, e, 0, bookworm, state.ZBattlefield)
	bear := moveSeededCard(t, e, 0, bearCard, state.ZBattlefield)
	e.pending = nil
	e.Advance()

	mark := len(e.L.Events)
	e.emit(events.Event{Kind: events.TurnFaceUp, Obj: bear})
	e.pending = nil
	e.Advance()

	if got := e.G.Obj(bookwormID).Imprinted; len(got) != 1 || got[0] != bear {
		t.Fatalf("Oblivious Bookworm Imprinted = %v, want [%d]: the TurnFaceUp Static$ True trigger must resolve off the stack",
			got, bear)
	}
	for _, ev := range e.L.Events[mark:] {
		if ev.Kind == events.TriggerPush && ev.Obj == bookwormID {
			t.Fatalf("the TurnFaceUp static trigger was pushed on the stack: %+v", ev)
		}
		if ev.Kind == events.DecisionAsk && ev.Text == string(decision.KTriggerOrder) {
			t.Fatalf("a static trigger entered the CR 603.3b order choice: %+v", ev)
		}
	}
	replayCheck(t, e, cfg)
}

// TestStaticTriggerUpkeepPhaseClearsImprintOffStack pins the second half:
// the Phase|Upkeep static clears the imprint at the NEXT upkeep (it has no
// ValidPlayer$, so it fires on every player's upkeep, and the next one is
// seat 1's), still without a stack object. The only Bookworm trigger ever
// pushed in the whole game is the end-step Draw, which is not static.
func TestStaticTriggerUpkeepPhaseClearsImprintOffStack(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	bookworm := mustCorpusCard(t, reg, "Oblivious Bookworm")
	bearCard := card(t, "Name:Test Bear\nTypes:Creature Bear\nPT:2/2\nOracle:x\n")
	e, cfg := tokenReplGame(t, 402, bookworm, bearCard)
	bookwormID := moveSeededCard(t, e, 0, bookworm, state.ZBattlefield)
	bear := moveSeededCard(t, e, 0, bearCard, state.ZBattlefield)
	e.pending = nil
	e.Advance()
	e.emit(events.Event{Kind: events.TurnFaceUp, Obj: bear})
	e.pending = nil
	e.Advance()
	if got := e.G.Obj(bookwormID).Imprinted; len(got) != 1 || got[0] != bear {
		t.Fatalf("setup: Imprinted = %v, want [%d]", got, bear)
	}

	for i := 0; i < 4000 && len(e.G.Obj(bookwormID).Imprinted) > 0; i++ {
		d := e.Pending()
		if d == nil {
			e.Advance()
			continue
		}
		switch {
		case d.Kind == decision.KPriority:
			passPriorityOnce(t, e)
		case d.Kind == decision.KTriggerOptional:
			// The end-step "you may draw" is NOT static: decline it, and it
			// must have been pushed (asserted below).
			idx := -1
			for _, o := range d.Options {
				if o.Kind == "no" {
					idx = o.Index
				}
			}
			if idx < 0 {
				t.Fatalf("optional trigger with no decline option: %+v", d)
			}
			if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{idx}}); err != nil {
				t.Fatalf("decline optional trigger: %v", err)
			}
		case d.Kind == decision.KAttackers || d.Kind == decision.KBlockers:
			if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: nil}); err != nil {
				t.Fatalf("submit %s: %v", d.Kind, err)
			}
		case d.Kind == decision.KChoose:
			if len(d.Options) == 0 {
				t.Fatalf("KChoose with no options: %+v", d)
			}
			if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{d.Options[0].Index}}); err != nil {
				t.Fatalf("submit %s: %v", d.Kind, err)
			}
		default:
			t.Fatalf("unexpected decision driving to the next upkeep: %+v", d)
		}
	}
	if got := e.G.Obj(bookwormID).Imprinted; len(got) != 0 {
		t.Fatalf("Oblivious Bookworm Imprinted = %v after the next upkeep, want empty: the Phase|Upkeep Static$ True trigger must resolve off the stack",
			got)
	}
	if e.G.Turn != 2 || e.G.Active != state.PlayerID(1) || e.G.Step != state.StepUpkeep {
		t.Fatalf("imprint cleared at turn %d seat %d step %s, want turn 2 seat 1 upkeep",
			e.G.Turn, e.G.Active, e.G.Step)
	}
	if n := countTriggerPushes(e, bookwormID); n != 1 {
		t.Fatalf("Bookworm stack pushes = %d, want 1 (the non-static end-step Draw trigger; both statics resolve off-stack)", n)
	}
	for _, ev := range e.L.Events {
		if ev.Kind == events.DecisionAsk && ev.Text == string(decision.KTriggerOrder) {
			t.Fatalf("a static trigger entered the CR 603.3b order choice: %+v", ev)
		}
	}
	replayCheck(t, e, cfg)
}

// TestStaticTriggerAskingBodyStaysOnStack is the conservative half: a
// Static$ True trigger whose body asks (Buzzing Whack-a-Doodle's
// ChoosePlayer/GenericChoice ETB) has no ask-free path off the stack, so it
// must still be pushed and resolved through the ordinary triggered-ability
// flow.
func TestStaticTriggerAskingBodyStaysOnStack(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	doodle := mustCorpusCard(t, reg, "Buzzing Whack-a-Doodle")
	e, cfg := tokenReplGame(t, 403, doodle)
	doodleID := moveSeededCard(t, e, 0, doodle, state.ZBattlefield)
	e.pending = nil
	e.Advance()

	if !hasEvent(e, events.TriggerPush, doodleID) {
		t.Fatal("the ETB Static$ True trigger with an asking body was not pushed on the stack")
	}
	if len(e.G.Stack) != 1 || e.G.Obj(e.G.Stack[0]).Source != doodleID {
		t.Fatalf("stack = %v, want the Whack-a-Doodle ETB trigger alone", e.G.Stack)
	}
	replayCheck(t, e, cfg)
}

// countTriggerPushes counts TriggerPush events naming obj as the source.
func countTriggerPushes(e *Engine, obj state.ObjID) int {
	n := 0
	for _, ev := range e.L.Events {
		if ev.Kind == events.TriggerPush && ev.Obj == obj {
			n++
		}
	}
	return n
}
