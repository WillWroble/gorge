package rules

import (
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
	"strings"
	"testing"
)

// Infernal Vessel returns from the graveyard as "a Demon in addition to its
// other types"; the rules derive Demon and the seat view's type line must
// show it (ViewTypes used to project only card-type changes).
func TestInfernalVesselReturnViewShowsDemon(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Infernal Vessel")
	v := moveByName(t, e, 0, "Infernal Vessel", state.ZBattlefield)
	e.emit(events.Event{Kind: events.MoveZone, Obj: v, From: state.ZBattlefield, To: state.ZGraveyard})
	e.priorityRound()
	fdnCensusDrain(t, e)
	d := e.Derived(v)
	t.Logf("zone %v derived types %v counters P1P1=%d", e.G.Obj(v).Zone, d.Types, e.G.Obj(v).Counter("P1P1"))
	if e.G.Obj(v).Zone != state.ZBattlefield {
		t.Fatalf("Vessel did not return: zone %v", e.G.Obj(v).Zone)
	}
	vw := view.Project(e.G, e, 0, nil)
	for _, cv := range vw.Players[0].Battlefield {
		if cv.ID == v {
			t.Logf("view types %q", cv.Types)
			if !strings.Contains(cv.Types, "Demon") {
				t.Fatalf("view type line %q lacks Demon (derived %v)", cv.Types, d.Types)
			}
			return
		}
	}
	t.Fatal("Vessel not in the view")
}
