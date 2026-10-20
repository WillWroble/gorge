package atomic

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// fake is the staging half of rules.Engine on one decision.
type fake struct {
	d         *decision.Decision
	committed bool
}

func (f *fake) Pending() *decision.Decision { return f.d }
func (f *fake) Stage(i int) error           { f.d.Staged = append(f.d.Staged, i); return nil }
func (f *fake) Decline(o state.ObjID) error { f.d.Declined = append(f.d.Declined, o); return nil }
func (f *fake) Commit() error               { f.committed = true; return nil }

func TestCombatIsOneCreatureAtATime(t *testing.T) {
	// Two attackers (objects 7 and 3), each able to attack two defenders.
	d := &decision.Decision{Kind: decision.KAttackers, Max: 4, Options: []decision.Option{
		{Index: 0, Obj: 7, Player: 1}, {Index: 1, Obj: 7, Player: 2},
		{Index: 2, Obj: 3, Player: 1}, {Index: 3, Obj: 3, Player: 2},
	}}
	f := &fake{d: d}
	a, done := Next(d)
	if done || a.Subject != 3 || a.Type != ChooseTarget || !a.CanStop || len(a.Options) != 2 {
		t.Fatalf("first atom %+v done=%v, want creature 3's two attacks or none", a, done)
	}
	if err := Apply(f, a, Stop); err != nil || f.committed {
		t.Fatalf("declining creature 3: err=%v committed=%v", err, f.committed)
	}
	a, _ = Next(d)
	if a.Subject != 7 {
		t.Fatalf("second atom is for %d, want 7", a.Subject)
	}
	if err := Apply(f, a, 1); err != nil || !f.committed {
		t.Fatalf("attacking with 7 completes the declaration: err=%v committed=%v", err, f.committed)
	}
}

func TestMultiPickStopsOnlyFromMin(t *testing.T) {
	d := &decision.Decision{Kind: decision.KTarget, Min: 1, Max: 2, Options: []decision.Option{{Index: 0, Obj: 1}, {Index: 1, Obj: 2}, {Index: 2, Obj: 3}}}
	a, _ := Next(d)
	if a.CanStop || len(a.Options) != 3 {
		t.Fatalf("before the minimum: %+v", a)
	}
	d.Staged = []int{1}
	a, _ = Next(d)
	if !a.CanStop || len(a.Options) != 2 {
		t.Fatalf("at the minimum: %+v", a)
	}
	d.Staged = []int{0, 1}
	if _, done := Next(d); !done {
		t.Fatal("at the maximum the answer is complete")
	}
}

func TestTypes(t *testing.T) {
	for _, c := range []struct {
		d    decision.Decision
		want Type
	}{
		{decision.Decision{Kind: decision.KPriority, Min: 1, Max: 1, Options: []decision.Option{{Kind: "pass"}}}, Priority},
		{decision.Decision{Kind: decision.KModes, Min: 1, Max: 1, Options: []decision.Option{{Kind: "mode"}}}, ChooseNum},
		{decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{{Kind: "x"}}}, ChooseNum},
		{decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{{Kind: "yes"}, {Kind: "no"}}}, ChooseUse},
		{decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{{Kind: "type"}}}, MakeChoice},
		{decision.Decision{Kind: decision.KChoose, Min: 1, Max: 1, Options: []decision.Option{{Kind: "sacrifice"}}}, ChooseTarget},
		{decision.Decision{Kind: decision.KTriggerOptional, Min: 1, Max: 1, Options: []decision.Option{{Kind: "yes"}, {Kind: "no"}}}, ChooseUse},
		{decision.Decision{Kind: decision.KTriggerOrder, Min: 2, Max: 2, Options: []decision.Option{{}, {}}}, ChooseTarget},
	} {
		if a, _ := Next(&c.d); a.Type != c.want {
			t.Errorf("%s %v: got %s, want %s", c.d.Kind, c.d.Options, a.Type, c.want)
		}
	}
}
