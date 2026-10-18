package rules

import (
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
	"strings"
	"testing"
)

// Sylvan Scavenging's Raccoon mode: ConditionPresent$ Creature.powerGE4+
// YouCtrl reads the creature's CURRENT power. Beast-Kin Ranger (3/3) pumped
// to 4 by its own trigger satisfies it at the end step; the condition count
// used to read printed power and created no Raccoon.
func TestSylvanScavengingReadsDerivedPower(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Sylvan Scavenging", "Beast-Kin Ranger", "Grizzly Bears")
	moveByName(t, e, 0, "Sylvan Scavenging", state.ZBattlefield)
	ranger := moveByName(t, e, 0, "Beast-Kin Ranger", state.ZBattlefield)
	moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield) // Ranger: +1/+0 until end of turn
	e.priorityRound()
	fdnCensusDrain(t, e)
	if p := e.Power(ranger); p != 4 {
		t.Fatalf("Ranger power %d after a creature entered, want 4", p)
	}
	turn := e.G.Turn
	for i := 0; i < 80 && e.G.Turn == turn; i++ {
		d := e.Pending()
		if d == nil {
			t.Fatal("no decision")
		}
		if d.Kind == decision.KPriority {
			submitPass(t, e)
			continue
		}
		pick := 0
		for _, o := range d.Options {
			if strings.Contains(o.Label, "Raccoon") || strings.Contains(o.Label, "Create") {
				pick = o.Index
			}
		}
		if d.Kind == decision.KChoose && d.ResumeKind == "discard" {
			submitChoices(t, e, 0)
			continue
		}
		submitChoices(t, e, pick)
	}
	raccoons := 0
	for _, id := range e.G.Zone(state.ZBattlefield, 0) {
		if o := e.G.Obj(id); o.IsToken && o.Face() != nil && strings.Contains(o.Face().Name, "Raccoon") {
			raccoons++
		}
	}
	if raccoons != 1 {
		t.Fatalf("raccoons %d, want 1 (a 4-power Ranger was controlled at the end step)", raccoons)
	}
}
