package rules

import (
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
	"strings"
	"testing"
)

// Kykar, Zephyr Awakener's blink: "Exile another target creature you
// control. Return that card to the battlefield ... at the beginning of the
// next end step." The delayed trigger's RememberObjects$ RememberedLKI is the
// exiled creature only. The SpellCast trigger's capture -- the noncreature
// spell that triggered it -- used to ride along, so a flashback Think Twice
// (already exiled) was returned to the battlefield at the end step.
func TestKykarBlinkReturnsOnlyTheExiledCreature(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Kykar, Zephyr Awakener", "Grizzly Bears", "Think Twice")
	moveByName(t, e, 0, "Kykar, Zephyr Awakener", state.ZBattlefield)
	bears := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)
	tt := moveByName(t, e, 0, "Think Twice", state.ZGraveyard)
	addMana(t, e, 0, "UCC")
	submitChoices(t, e, castModeOption(t, e, tt, "flashback"))
	turn := e.G.Turn
	for i := 0; i < 80 && e.G.Turn == turn; i++ {
		d := e.Pending()
		if d == nil {
			t.Fatal("no decision")
		}
		switch d.Kind {
		case decision.KPriority:
			submitPass(t, e)
		case decision.KModes:
			pick := 0
			for _, o := range d.Options {
				if strings.Contains(o.Label, "Exile another") {
					pick = o.Index
				}
			}
			submitChoices(t, e, pick)
		case decision.KTarget:
			answerTargetAsk(t, e, []state.ObjID{bears})
		default:
			fdnCensusAnswerMin(t, e, d)
		}
		if e.G.Step == state.StepCleanup {
			break
		}
	}
	t.Logf("Think Twice zone %v, Bears zone %v", e.G.Obj(tt).Zone, e.G.Obj(bears).Zone)
	if e.G.Obj(bears).Zone != state.ZBattlefield {
		t.Fatalf("Bears not returned: %v", e.G.Obj(bears).Zone)
	}
	if z := e.G.Obj(tt).Zone; z != state.ZExile {
		t.Fatalf("flashback Think Twice in %v after Kykar's return, want exile", z)
	}
}
