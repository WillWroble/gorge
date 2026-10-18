package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// A Witness-Protected Gnarlid Colony loses "Each creature you control with a
// +1/+1 counter on it has trample". Both effects apply in layer 6, but the
// grant's existence depends on the ability removal (CR 613.8a), so the
// removal applies first whatever the timestamps and the grant never applies:
// neither the Colony nor another countered creature has trample (census
// pairs 4109, 6841).
func TestWitnessProtectedColonyGrantsNoTrample(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e, cfg := corpusEngineCfg(t, reg, []*cards.Card{lookup(t, reg, "Gnarlid Colony"), lookup(t, reg, "Grizzly Bears"),
		lookup(t, reg, "Witness Protection")}, nil)
	colony := moveCorpusCard(t, e, "Gnarlid Colony", 0, state.ZBattlefield)
	bears := moveCorpusCard(t, e, "Grizzly Bears", 0, state.ZBattlefield)
	e.emit(events.Event{Kind: events.CounterChange, Obj: colony, Counter: "P1P1", Amount: 2})
	e.emit(events.Event{Kind: events.CounterChange, Obj: bears, Counter: "P1P1", Amount: 1})
	lossNextTurn(e)
	if !e.HasKeyword(colony, "Trample") || !e.HasKeyword(bears, "Trample") {
		t.Fatalf("precondition: Colony trample=%v Bears trample=%v, want both", e.HasKeyword(colony, "Trample"), e.HasKeyword(bears, "Trample"))
	}
	lossAttach(t, e, "Witness Protection", 0, colony)
	if e.HasKeyword(colony, "Trample") {
		t.Errorf("the Witness-Protected Colony still has trample: %v", e.Keywords(colony))
	}
	if e.HasKeyword(bears, "Trample") {
		t.Errorf("Grizzly Bears still has trample from a Colony with no abilities: %v", e.Keywords(bears))
	}
	replayCheck(t, e, cfg)
}
