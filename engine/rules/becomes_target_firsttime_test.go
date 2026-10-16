package rules

// Mode$ BecomesTarget with FirstTime$ True ("becomes the target of a spell or
// ability for the first time each turn"): Glyph Keeper, Kira, the Glasskites,
// Valiant creatures. The gate is trigmatch.firstTimeTargetedThisTurn.

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/internal/testutil"
)

func TestBecomesTargetFirstTimeCountersOnlyFirstTargetingEachTurn(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	trig := corpusTriggerMode(t, mustCorpusCard(t, reg, "Glyph Keeper"), "BecomesTarget")
	if trig.Params["FirstTime"] != "True" {
		t.Fatalf("corpus fixture drift: Glyph Keeper FirstTime$ = %q, want True", trig.Params["FirstTime"])
	}
	e, ids := becomesTargetOnceBoard(t, reg,
		[]string{"Glyph Keeper", "Prodigal Pyromancer", "Prodigal Pyromancer", "Prodigal Pyromancer"},
		nil)
	keeper := ids["Glyph Keeper"]
	var pyros []state.ObjID
	for _, id := range e.G.Zone(state.ZBattlefield, 0) {
		if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Prodigal Pyromancer" {
			pyros = append(pyros, id)
		}
	}
	if len(pyros) != 3 {
		t.Fatalf("precondition: %d Pyromancers, want 3", len(pyros))
	}
	if o := e.G.Obj(keeper); o == nil || o.Zone != state.ZBattlefield || o.Damage != 0 {
		t.Fatalf("precondition: Glyph Keeper not an undamaged battlefield permanent: %+v", o)
	}
	ping := func(p state.ObjID) {
		t.Helper()
		if o := e.G.Obj(p); o == nil || o.Tapped || o.Zone != state.ZBattlefield {
			t.Fatalf("precondition: source %d not an untapped battlefield permanent: %+v", p, o)
		}
		e.askPriority(0)
		submitChoices(t, e, abilityOption(t, e, p, 0).Index)
		submitChoices(t, e, targetOptionIdx(t, e, keeper))
		passUntilStackEmpty(t, e, 40)
	}
	ping(pyros[0])
	if got := e.G.Obj(keeper).Damage; got != 0 {
		t.Fatalf("first targeting this turn was not countered: Keeper damage = %d, want 0", got)
	}
	ping(pyros[1])
	if got := e.G.Obj(keeper).Damage; got != 1 {
		t.Fatalf("second targeting this turn was countered: Keeper damage = %d, want 1", got)
	}
	// A new turn re-arms "the first time each turn": the log scan stops at
	// TurnChange, so a third ping on a later turn is countered again.
	e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: e.G.Turn + 1})
	e.G.Obj(keeper).Damage = 0
	e.G.Step = state.StepMain1
	ping(pyros[2])
	if got := e.G.Obj(keeper).Damage; got != 0 {
		t.Fatalf("first targeting of the next turn was not countered: Keeper damage = %d, want 0", got)
	}
}
