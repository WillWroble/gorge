package pay

import (
	"slices"
	"testing"

	costvocab "github.com/adams-shaun/gorge/rules/cost"
)

func TestGroupSums(t *testing.T) {
	if got := GroupSums([]int32{2, 1}, 1); !slices.Equal(got, []int32{1, 2, 3}) {
		t.Errorf("GroupSums(2,1 / >=1) = %v, want 1 2 3", got)
	}
	// A zero-value member: the empty set is not an election of one or more.
	if got := GroupSums([]int32{0, 3}, 1); !slices.Equal(got, []int32{0, 3}) {
		t.Errorf("GroupSums(0,3 / >=1) = %v, want 0 3", got)
	}
	if got := GroupSums([]int32{1, 1}, 2); !slices.Equal(got, []int32{2}) {
		t.Errorf("GroupSums(1,1 / >=2) = %v, want 2", got)
	}
	if got := GroupSums(nil, 1); len(got) != 0 {
		t.Errorf("GroupSums(none) = %v", got)
	}
}

func TestGroupFeasible(t *testing.T) {
	ge := costvocab.GroupTotal{Prop: costvocab.GroupManaSymbols, Color: 'B', Cmp: costvocab.GroupGE, N: 15}
	if !GroupFeasible([]int32{4, 4, 5, 4}, 1, ge, 15) || GroupFeasible([]int32{4, 4, 4}, 1, ge, 15) {
		t.Error("GE 15 floor misjudged")
	}
	eq := costvocab.GroupTotal{Prop: costvocab.GroupCMC, Cmp: costvocab.GroupEQ, Var: "X"}
	if !GroupFeasible([]int32{2, 1}, 1, eq, 3) || GroupFeasible([]int32{2, 1}, 1, eq, 4) {
		t.Error("EQ X total misjudged")
	}
	if GroupFeasible(nil, 1, costvocab.GroupTotal{}, 0) {
		t.Error("an inactive group with too few candidates is feasible")
	}
}
