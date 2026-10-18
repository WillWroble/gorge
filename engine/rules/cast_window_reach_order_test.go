package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// CR 601.2c, 602.2b: Zimone, Paradox Sculptor's "{G}{U}, {T}: double the
// counters on up to two target creatures and/or artifacts you control" asks
// for its targets before payment. Its only {G} source is Blossoming Sands,
// the first source in the cast window's walk, whose first production is
// {W}; with nine more untapped lands the reachability search used to spend
// its node cap enumerating land orders before trying the Sands' {G}, judged
// the payable cost unreachable for every target, and the Min 0 ask was
// skipped (lockstep pairs 1509 3986).
func TestCastWindowReachableWithManyLandsKeepsTargets(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	board := []string{"Blossoming Sands", "Zimone, Paradox Sculptor", "Grizzly Bears",
		"Island", "Island", "Island", "Island", "Island", "Plains", "Plains", "Plains", "Plains"}
	eng, cfg := miscHandsEngine(t, reg, nil, nil, board, nil)
	z := miscBoardObj(t, eng, 0, "Zimone, Paradox Sculptor")
	// Two turns on, Zimone can tap (its begin-combat trigger is answered
	// with no targets on the way).
	goal := eng.G.Turn + 2
	for i := 0; ; i++ {
		if i > 400 {
			t.Fatal("never reached the next own main phase")
		}
		d := eng.Pending()
		if eng.G.Turn == goal && eng.G.Step == state.StepMain1 && d.Kind == decision.KPriority {
			break
		}
		if d.Kind == decision.KPriority {
			submitPass(t, eng)
		} else {
			fdnCensusAnswerMin(t, eng, d)
		}
	}
	if eng.G.Players[0].Pool != (state.Mana{}) {
		t.Fatalf("precondition: seat 0's pool is %v, want empty (the lands pay)", eng.G.Players[0].Pool)
	}
	idx := -1
	for _, o := range eng.Pending().Options {
		if o.Kind == "ability" && o.Obj == z {
			idx = o.Index
		}
	}
	if idx < 0 {
		t.Fatal("precondition: Zimone's ability is not offered")
	}
	submitChoices(t, eng, idx)
	d := eng.Pending()
	if d == nil || d.Kind != decision.KTarget || d.Min != 0 || d.Max != 2 {
		t.Fatalf("after activating Zimone: %+v (stack %d), want its up-to-two target ask", d, len(eng.G.Stack))
	}
	if len(d.Options) != 2 {
		t.Fatalf("Zimone's target ask offers %d options, want Zimone and the Bears", len(d.Options))
	}
	submitChoices(t, eng, d.Options[1].Index)
	replayCheck(t, eng, cfg)
}
