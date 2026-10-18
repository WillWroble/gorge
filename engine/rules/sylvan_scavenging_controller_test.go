package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestSylvanScavengingConditionReadsItsController pins Sylvan Scavenging's
// Raccoon mode (ConditionPresent$ Creature.powerGE4+YouCtrl) to the
// trigger's controller when that is seat 1: "you" is the controller of the
// resolving ability (CR 109.5), never seat 0. Census pairs 77, 253, 378.
func TestSylvanScavengingConditionReadsItsController(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		big, bear state.PlayerID
		want      int
	}{
		{name: "seat 1 controls the 4-power creature", big: 1, bear: 0, want: 1},
		{name: "only seat 0 controls one", big: 0, bear: 1, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := testutil.CorpusRegistry(t)
			big := mustCorpusCard(t, reg, "Gigantosaurus")
			bear := mustCorpusCard(t, reg, "Grizzly Bears")
			scav := mustCorpusCard(t, reg, "Sylvan Scavenging")
			extras := [2][]*cards.Card{}
			extras[1] = append(extras[1], scav)
			extras[tc.big] = append(extras[tc.big], big)
			extras[tc.bear] = append(extras[tc.bear], bear)
			e := corpusEngine(t, reg, extras[0], extras[1])
			moveByName(t, e, 1, "Sylvan Scavenging", state.ZBattlefield)
			moveByName(t, e, tc.big, "Gigantosaurus", state.ZBattlefield)
			moveByName(t, e, tc.bear, "Grizzly Bears", state.ZBattlefield)
			start := e.G.Turn
			for i := 0; i < 300 && e.G.Turn < start+2 && !e.G.Over; i++ {
				d := e.Pending()
				if d == nil {
					t.Fatal("no decision")
				}
				switch d.Kind {
				case decision.KPriority:
					submitPass(t, e)
					continue
				case decision.KAttackers, decision.KBlockers:
					submitChoices(t, e)
					continue
				}
				pick := d.Options[0].Index
				for _, o := range d.Options {
					if strings.Contains(o.Label, "Raccoon") {
						pick = o.Index
					}
				}
				if d.Kind == decision.KChoose && d.ResumeKind == "discard" {
					pick = d.Options[0].Index
				}
				submitChoices(t, e, pick)
			}
			raccoons := 0
			for _, id := range e.G.Zone(state.ZBattlefield, 1) {
				if o := e.G.Obj(id); o.IsToken && o.Face() != nil && strings.Contains(o.Face().Name, "Raccoon") {
					raccoons++
				}
			}
			if raccoons != tc.want {
				t.Fatalf("seat 1 raccoons %d, want %d", raccoons, tc.want)
			}
		})
	}
}
