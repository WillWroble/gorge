package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/state"
)

// Ghalta, Primal Hunger costs {X} less, "where X is the total power of
// creatures you control" (Count$Valid Creature.YouCtrl$CardPower). A
// creature's power is its current power (CR 208.3): Glorious Anthem's +1/+1
// counts. The sum used to read the printed face, so the kernel offered a
// Ghalta cast gorge did not (census pairs 145, 1067).
func TestGhaltaCountsDerivedPower(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Ghalta, Primal Hunger", "Grizzly Bears", "Grizzly Bears", "Glorious Anthem")
	ghalta := moveByName(t, e, 0, "Ghalta, Primal Hunger", state.ZHand)
	moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	total := func() int32 {
		return effects.EvalCount(e, &effects.Ctx{Source: ghalta, Controller: 0}, "Count$Valid Creature.YouCtrl$CardPower")
	}
	if got := total(); got != 4 {
		t.Fatalf("two Grizzly Bears: total power %d, want 4", got)
	}
	moveByName(t, e, 0, "Glorious Anthem", state.ZBattlefield)
	if got := total(); got != 6 {
		t.Fatalf("two Grizzly Bears under Glorious Anthem: total power %d, want 6", got)
	}
}
