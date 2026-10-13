package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// lentCounterRiderToken puts `permanent` onto seat 0's battlefield and mints
// one token of `tokenScript` for seat 0 through the ordinary TokenCreate
// emit. When opponentLostLife is set an opponent is dealt damage first, so an
// "if an opponent lost life this turn" gate reads true. Returns the token.
func lentCounterRiderToken(t *testing.T, permanent, tokenScript string, opponentLostLife bool) (*Engine, *state.Object) {
	t.Helper()
	e := handEngineTokens(t, corpusAlternativeCard(t, permanent))
	id := e.G.Zone(state.ZHand, 0)[0]
	e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZHand, To: state.ZBattlefield})
	drainQueuedTriggers(t, e)
	if o := e.G.Obj(id); o == nil || o.Zone != state.ZBattlefield {
		t.Fatalf("precondition: %s did not enter: %+v", permanent, o)
	}
	if opponentLostLife {
		e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: e.G.Turn + 1})
		e.emit(events.Event{Kind: events.Damage, Player: 1, Amount: 2})
	}
	want := e.G.NextID
	e.EmitTokenCreate(events.Event{Kind: events.TokenCreate, Player: 0, Text: tokenScript})
	drainQueuedTriggers(t, e)
	tok := e.G.Obj(want)
	if tok == nil || tok.Zone != state.ZBattlefield || !tok.IsToken {
		t.Fatalf("precondition: the %s token was not created: %+v", tokenScript, tok)
	}
	return e, tok
}

// lentCounterRiderCard puts Vampire Socialite onto seat 0's battlefield, then
// moves a second Vampire card from hand onto the battlefield. When
// opponentLostLife is set an opponent is dealt damage before the second card
// enters. Returns the entering card.
func lentCounterRiderCard(t *testing.T, opponentLostLife bool) (*Engine, *state.Object) {
	t.Helper()
	e := handEngineTokens(t,
		corpusAlternativeCard(t, "Vampire Socialite"),
		corpusAlternativeCard(t, "Vampire Lacerator"))
	ids := e.G.Zone(state.ZHand, 0)
	if len(ids) != 2 {
		t.Fatalf("precondition: hand = %v, want Socialite + Lacerator", ids)
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: ids[0], From: state.ZHand, To: state.ZBattlefield})
	drainQueuedTriggers(t, e)
	if o := e.G.Obj(ids[0]); o == nil || o.Zone != state.ZBattlefield {
		t.Fatalf("precondition: Vampire Socialite did not enter: %+v", o)
	}
	if opponentLostLife {
		e.emit(events.Event{Kind: events.TurnChange, Player: 0, Amount: e.G.Turn + 1})
		e.emit(events.Event{Kind: events.Damage, Player: 1, Amount: 2})
	}
	enterID := e.G.Zone(state.ZHand, 0)[0]
	e.emit(events.Event{Kind: events.MoveZone, Obj: enterID, From: state.ZHand, To: state.ZBattlefield})
	drainQueuedTriggers(t, e)
	o := e.G.Obj(enterID)
	if o == nil || o.Zone != state.ZBattlefield {
		t.Fatalf("precondition: the Vampire card did not enter: %+v", o)
	}
	return e, o
}

// TestEntryLentCounterRiderHonoursConditionGate: a lent ETB counter rider
// whose ConditionCheckSVar$ is false must place nothing, exactly as the live
// Resolve path skips the body -- the entry fold must not absorb it and add its
// count unconditionally (Vampire Socialite's "as long as an opponent lost
// life this turn"). With the gate true the same rider places exactly one.
func TestEntryLentCounterRiderHonoursConditionGate(t *testing.T) {
	t.Parallel()
	_, tok := lentCounterRiderToken(t, "Vampire Socialite", "b_1_1_vampire", false)
	if got := tok.Counter("P1P1"); got != 0 {
		t.Fatalf("condition false: token +1/+1 counters = %d, want 0", got)
	}
	_, tok2 := lentCounterRiderToken(t, "Vampire Socialite", "b_1_1_vampire", true)
	if got := tok2.Counter("P1P1"); got != 1 {
		t.Fatalf("condition true: token +1/+1 counters = %d, want 1", got)
	}
}

// TestEntryLentCounterRiderHonoursConditionGateOnCard is the card-entry half:
// the same rider on a card moving onto the battlefield.
func TestEntryLentCounterRiderHonoursConditionGateOnCard(t *testing.T) {
	t.Parallel()
	_, o := lentCounterRiderCard(t, false)
	if got := o.Counter("P1P1"); got != 0 {
		t.Fatalf("condition false: card +1/+1 counters = %d, want 0", got)
	}
	_, o2 := lentCounterRiderCard(t, true)
	if got := o2.Counter("P1P1"); got != 1 {
		t.Fatalf("condition true: card +1/+1 counters = %d, want 1", got)
	}
}

// TestEntryTokenLentRiderWithHardenedScales: a creature token a lent rider
// (Grumgully) augments and Hardened Scales then modifies enters with two
// +1/+1 counters, not three -- the rider's placement and the token's entry
// counters are one event (CR 614.1c/122.6), so the modifier applies once.
func TestEntryTokenLentRiderWithHardenedScales(t *testing.T) {
	t.Parallel()
	e := handEngineTokens(t,
		corpusAlternativeCard(t, "Grumgully, the Generous"),
		corpusAlternativeCard(t, "Hardened Scales"))
	for _, id := range e.G.Zone(state.ZHand, 0) {
		e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZHand, To: state.ZBattlefield})
		drainQueuedTriggers(t, e)
		if o := e.G.Obj(id); o == nil || o.Zone != state.ZBattlefield {
			t.Fatalf("precondition: lent-rider permanent did not enter: %+v", o)
		}
	}
	want := e.G.NextID
	e.EmitTokenCreate(events.Event{Kind: events.TokenCreate, Player: 0, Text: "r_1_1_goblin"})
	drainQueuedTriggers(t, e)
	tok := e.G.Obj(want)
	if tok == nil || tok.Zone != state.ZBattlefield || !tok.IsToken {
		t.Fatalf("precondition: the Goblin token was not created: %+v", tok)
	}
	if got := tok.Counter("P1P1"); got != 2 {
		t.Fatalf("token +1/+1 counters = %d, want 2 (Grumgully's rider plus Hardened Scales, once)", got)
	}
}
