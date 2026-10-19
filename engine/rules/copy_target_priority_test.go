package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// CR 117.3b: after a copy resolves, the active player receives priority with
// no passes counted, also when the copy's resolution first asked its
// controller for new targets (CR 707.10c). Seat 1 answers seat 0's Lightning
// Bolt with Reverberate; seat 1 controls the copy and is the last to pass
// before it resolves. The copy-target answer resolved the copy in line and
// used to leave priority with seat 1, one pass already counted, so seat 1's
// next pass resolved the Bolt without seat 0 ever receiving priority
// (lockstep pair 6042, Thousand-Year Storm).
func TestCopyTargetAskResolutionGivesActivePlayerPriority(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	eng, cfg := miscHandsEngine(t, reg,
		[]string{"Lightning Bolt"}, []string{"Reverberate"}, nil, nil)
	addMana(t, eng, 0, "R")
	addMana(t, eng, 1, "RR")
	bolt := miscHandObj(t, eng, 0, "Lightning Bolt")
	submitChoices(t, eng, miscCastOption(t, eng, bolt))
	answerPlayerTarget := func(p state.PlayerID) {
		t.Helper()
		d := eng.Pending()
		if d == nil || d.Kind != decision.KTarget {
			t.Fatalf("expected a target ask, got %+v", d)
		}
		for _, o := range d.Options {
			if o.Kind == "player" && o.Player == p {
				submitChoices(t, eng, o.Index)
				return
			}
		}
		t.Fatalf("seat %d not offered: %+v", p, d.Options)
	}
	answerPlayerTarget(1)
	submitPass(t, eng) // seat 0 passes with the Bolt on the stack
	rev := miscHandObj(t, eng, 1, "Reverberate")
	submitChoices(t, eng, miscCastOption(t, eng, rev))
	if d := eng.Pending(); d != nil && d.Kind == decision.KTarget {
		for _, o := range d.Options {
			if o.Obj == bolt {
				submitChoices(t, eng, o.Index)
				break
			}
		}
	}
	// Reverberate resolves, then the copy: seat 1 passes last each time.
	d := passUntilNonPriority(t, eng, 10)
	if d == nil || d.Kind != decision.KTarget || d.ResumeKind != "copy_targets" {
		t.Fatalf("expected the copy-target ask, got %+v", d)
	}
	if eng.G.Active != 0 || eng.G.Priority != 1 {
		t.Fatalf("precondition: active %d, last passer %d; want seat 1 to pass last in seat 0's turn", eng.G.Active, eng.G.Priority)
	}
	answerPlayerTarget(0)
	if len(eng.G.Stack) != 1 || eng.G.Stack[0] != bolt {
		t.Fatalf("precondition: stack %v after the copy resolved, want only the Bolt (%d); pending %+v", eng.G.Stack, bolt, eng.Pending())
	}
	d = eng.Pending()
	if d == nil || d.Kind != decision.KPriority || d.Player != eng.G.Active || eng.G.Passes != 0 {
		who := -1
		if d != nil {
			who = int(d.Player)
		}
		t.Fatalf("after the copy resolved: %+v to seat %d with %d pass(es) counted, want priority to the active seat %d with none (CR 117.3b)",
			d, who, eng.G.Passes, eng.G.Active)
	}
	replayCheck(t, eng, cfg)
}
