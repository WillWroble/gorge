package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

// squandered builds seat 0's battlefield: Squandered Resources plus the given
// land scripts (each a full card script), all untapped.
func squanderedBoard(t *testing.T, lands ...string) (*Engine, state.ObjID, []state.ObjID) {
	t.Helper()
	e := handEngine(t, corpusAlternativeCard(t, "Squandered Resources"))
	id := e.G.Zone(state.ZHand, 0)[0]
	e.G.Obj(id).Zone = state.ZBattlefield
	bf := []state.ObjID{id}
	var ls []state.ObjID
	for _, src := range lands {
		l := e.G.AddObject(card(t, src), 0)
		l.Zone = state.ZBattlefield
		bf = append(bf, l.ID)
		ls = append(ls, l.ID)
	}
	e.G.SetZone(state.ZHand, 0, nil)
	e.G.SetZone(state.ZBattlefield, 0, bf)
	if o := e.G.Obj(id); o.Zone != state.ZBattlefield || o.Face() == nil || o.Face().Name != "Squandered Resources" {
		t.Fatalf("fixture: Squandered Resources not on the battlefield: %+v", o)
	}
	e.priorityRound()
	return e, id, ls
}

const squanderedForest = "Name:Probe Forest\nTypes:Land\nA:AB$ Mana | Cost$ T | Produced$ G\nOracle:x\n"

// A ManaReflected ability whose Valid$ is Defined.Sacrificed reads a
// permanent that does not exist until its Sac cost is paid. The offer gate
// used to read the empty Remembered list, found no candidate, and never
// offered Squandered Resources (corpus census: IN_DECK 4 games, OFFERED 0).
func TestSquanderedResourcesOfferedAndSacrificesLandForItsMana(t *testing.T) {
	t.Parallel()
	e, id, ls := squanderedBoard(t, squanderedForest)
	if !hasActivateOption(e, id) {
		t.Fatalf("Squandered Resources not offered with a mana-producing land to sacrifice: %+v", e.Pending().Options)
	}
	for _, o := range e.Pending().Options {
		if o.Kind == "activate" && o.Obj == id {
			submitChoices(t, e, o.Index)
			break
		}
	}
	// Any sacrifice choice the cost asks for takes the one land.
	for i := 0; i < 4 && e.Pending() != nil && e.Pending().Kind != decision.KPriority; i++ {
		submitChoices(t, e, 0)
	}
	if got := e.G.Obj(ls[0]).Zone; got != state.ZGraveyard {
		t.Fatalf("land zone = %v, want the graveyard (sacrificed as the cost)", got)
	}
	if got := e.G.Players[0].Pool[state.MG]; got != 1 {
		t.Fatalf("pool G = %d, want 1 (the sacrificed Forest-alike's colour)", got)
	}
}

// The probe must be able to fail: with no land that could reflect a mana (no
// land at all, or a land with no mana ability) the ability stays unoffered.
func TestSquanderedResourcesNotOfferedWithoutAReflectingLand(t *testing.T) {
	t.Parallel()
	e, id, _ := squanderedBoard(t)
	if hasActivateOption(e, id) {
		t.Fatalf("offered with no land to sacrifice")
	}
	e2, id2, ls := squanderedBoard(t, "Name:Probe Blank\nTypes:Land\nOracle:x\n")
	if o := e2.G.Obj(ls[0]); o.Zone != state.ZBattlefield {
		t.Fatalf("fixture: blank land not on the battlefield")
	}
	if hasActivateOption(e2, id2) {
		t.Fatalf("offered although the only land could produce no mana")
	}
}
