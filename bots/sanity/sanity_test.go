package sanity

import (
	"context"
	"reflect"
	"testing"

	"github.com/adams-shaun/gorge/bots/heuristic"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
)

func hostileOwnOnly() decision.TargetHints { return decision.MakeTargetHints(2, 0, true) }

func prio() decision.Decision {
	i1, i2 := 1, 2
	return decision.Decision{
		Kind: decision.KPriority, Seq: 7, Min: 1, Max: 1,
		Options: []decision.Option{
			{Index: 0, Kind: "play_land", Label: "Play Forest"},
			{Index: 1, Kind: "cast", Label: "Cast Swords to Plowshares", TargetHints: hostileOwnOnly()},
			{Index: 2, Kind: "cast", Label: "Cast Lightning Bolt", TargetHints: decision.MakeTargetHints(1, 1, true)},
			{Index: 3, Kind: "ability", Label: "Wasteland: Destroy target nonbasic land", TargetHints: hostileOwnOnly()},
			{Index: 4, Kind: "cast", Label: "Cast Ghost Quarter", TargetHints: hostileOwnOnly()},
			{Index: 5, Kind: "pass", Label: "Pass priority"},
			{Index: 6, Kind: "concede", Label: "Concede"},
		},
		PaymentActions: []decision.PaymentAction{
			{ID: "a1", Label: "Cast Swords to Plowshares", BaseOptionIndex: &i1},
			{ID: "a2", Label: "Cast Lightning Bolt", BaseOptionIndex: &i2},
			{ID: "a3", Label: "Cast Path to Exile", TargetHints: hostileOwnOnly()},
			{ID: "a4", Label: "Cast Doom Blade", TargetHints: decision.MakeTargetHints(1, 3, true)},
		},
	}
}

func TestFilterRenumbersAndRemaps(t *testing.T) {
	f := Filter(prio())
	var kinds []string
	for i, o := range f.D.Options {
		if o.Index != i {
			t.Fatalf("option %d has Index %d", i, o.Index)
		}
		kinds = append(kinds, o.Label)
	}
	want := []string{"Play Forest", "Cast Lightning Bolt", "Cast Ghost Quarter", "Pass priority", "Concede"}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kept %v, want %v", kinds, want)
	}
	if !reflect.DeepEqual(f.Keep, []int{0, 2, 4, 5, 6}) {
		t.Fatalf("keep = %v", f.Keep)
	}
	var ids []string
	for _, pa := range f.D.PaymentActions {
		ids = append(ids, pa.ID)
		if pa.ID == "a2" && (pa.BaseOptionIndex == nil || *pa.BaseOptionIndex != 1) {
			t.Fatalf("a2 base = %v, want remapped to 1", pa.BaseOptionIndex)
		}
	}
	if !reflect.DeepEqual(ids, []string{"a2", "a4"}) {
		t.Fatalf("kept actions %v: a1 (linked to a dropped option) and a3 (own hint) must go", ids)
	}
	// The input is untouched.
	if d := prio(); len(d.Options) != 7 || len(d.PaymentActions) != 4 {
		t.Fatal("fixture drifted")
	}
	in := f.MapBack(decision.Intent{Choices: []int{1}})
	if in.Choices[0] != 2 {
		t.Fatalf("MapBack(1) = %v, want original index 2", in.Choices)
	}
	if got := f.MapIn(decision.Intent{Choices: []int{2}}); got.Choices[0] != 1 {
		t.Fatalf("MapIn(2) = %v", got.Choices)
	}
}

func TestFilterIsIdentityWithoutDrops(t *testing.T) {
	d := prio()
	d.Options[1].TargetHints = 0
	d.Options[3].TargetHints = 0
	d.PaymentActions = d.PaymentActions[1:2]
	f := Filter(d)
	if f.Keep != nil || len(f.D.Options) != len(d.Options) {
		t.Fatal("nothing hinted for dropping, yet the decision changed")
	}
	nd := decision.Decision{Kind: decision.KTarget, Options: []decision.Option{{Kind: "cast", TargetHints: hostileOwnOnly()}}}
	if Filter(nd).Keep != nil {
		t.Fatal("a non-priority decision was filtered")
	}
}

func TestDropsNeedsOwnTargetsAndNoOpponent(t *testing.T) {
	for _, tc := range []struct {
		name string
		h    decision.TargetHints
		want bool
	}{
		{"no hint", 0, false},
		{"own only", decision.MakeTargetHints(1, 0, true), true},
		{"both", decision.MakeTargetHints(1, 1, true), false},
		{"opposing only", decision.MakeTargetHints(0, 2, true), false},
		{"no targets at all", decision.MakeTargetHints(0, 0, true), false},
		{"not hostile", decision.MakeTargetHints(1, 0, false), false},
	} {
		o := decision.Option{Kind: "cast", Label: "Cast X", TargetHints: tc.h}
		if got := Drops(&o); got != tc.want {
			t.Errorf("%s: Drops = %v, want %v", tc.name, got, tc.want)
		}
	}
	if Drops(&decision.Option{Kind: "cast", Label: "Cast Ghost Quarter (kicked)", TargetHints: hostileOwnOnly()}) {
		t.Error("an allow-listed card was dropped")
	}
	if Drops(&decision.Option{Kind: "pass", TargetHints: hostileOwnOnly()}) {
		t.Error("pass was dropped")
	}
}

func TestAllowListSortedAndNames(t *testing.T) {
	for i := 1; i < len(allowList); i++ {
		if allowList[i-1] >= allowList[i] {
			t.Fatalf("allowList not strictly sorted at %d: %q >= %q", i, allowList[i-1], allowList[i])
		}
	}
	for label, want := range map[string]string{
		"Cast Swords to Plowshares":    "Swords to Plowshares",
		"Cast Whacker (kicked)":        "Whacker",
		"Wasteland: Destroy target...": "Wasteland",
	} {
		kind := "cast"
		if label[0] != 'C' || label == "Cast" {
			kind = "ability"
		}
		if label == "Wasteland: Destroy target..." {
			kind = "ability"
		}
		if got := cardName(&decision.Option{Kind: kind, Label: label}); got != want {
			t.Errorf("cardName(%q) = %q, want %q", label, got, want)
		}
	}
}

// recSeat records the decision it is asked and picks the first option of the
// given label.
type recSeat struct {
	got  decision.Decision
	pick string
}

func (r *recSeat) Decide(_ context.Context, _ view.View, d decision.Decision) (decision.Intent, error) {
	r.got = d
	for _, o := range d.Options {
		if o.Label == r.pick {
			return decision.Intent{Seq: d.Seq, Choices: []int{o.Index}}, nil
		}
	}
	return decision.Intent{}, nil
}

type recBoard struct{ recSeat }

func (r *recBoard) DecideBoard(ctx context.Context, _ botpolicy.Board, d decision.Decision) (decision.Intent, error) {
	return r.Decide(ctx, view.View{}, d)
}

func TestWrapAsksOnceAndMapsBack(t *testing.T) {
	inner := &recSeat{pick: "Cast Lightning Bolt"}
	w := Wrap(inner)
	if _, ok := w.(heuristic.BoardSeat); ok {
		t.Fatal("a non-board inner became a BoardSeat")
	}
	if c, ok := w.(seat.TargetHintConsumer); !ok || !c.WantsTargetHints() {
		t.Fatal("the wrapper must request target hints")
	}
	in, err := w.Decide(context.Background(), view.View{}, prio())
	if err != nil {
		t.Fatal(err)
	}
	if len(inner.got.Options) != 5 {
		t.Fatalf("inner saw %d options, want the filtered 5", len(inner.got.Options))
	}
	if in.Choices[0] != 2 {
		t.Fatalf("answer maps to original option %d, want 2 (Lightning Bolt)", in.Choices[0])
	}

	b := &recBoard{recSeat{pick: "Cast Ghost Quarter"}}
	wb := Wrap(b)
	bs, ok := wb.(heuristic.BoardSeat)
	if !ok {
		t.Fatal("a BoardSeat inner must stay a BoardSeat")
	}
	in, err = bs.DecideBoard(context.Background(), botpolicy.Board{}, prio())
	if err != nil || in.Choices[0] != 4 {
		t.Fatalf("board answer %v err %v, want original index 4", in.Choices, err)
	}
	if u, ok := wb.(interface{ UnwrapSeat() seat.Seat }); !ok || u.UnwrapSeat() != seat.Seat(b) {
		t.Fatal("UnwrapSeat must expose the wrapped seat")
	}
}

// TestWrapAnswersMulliganItself pins that a keep/mulligan ask never reaches
// the wrapped seat (its rng is not touched) and that a Board with a
// no-land seven is answered "mulligan" while a seven with three lands keeps.
func TestWrapAnswersMulliganItself(t *testing.T) {
	land := func() botpolicy.Card {
		return botpolicy.Card{Basic: true, ManaCost: "no cost", Castable: true}
	}
	spell := func() botpolicy.Card {
		return botpolicy.Card{Creature: true, CMC: 2, ManaCost: "2", Castable: true}
	}
	hand := func(lands int) botpolicy.Board {
		m := map[state.ObjID]botpolicy.Card{}
		for i := 0; i < 7; i++ {
			if i < lands {
				m[state.ObjID(i+1)] = land()
			} else {
				m[state.ObjID(i+1)] = spell()
			}
		}
		return botpolicy.Board{Cards: botpolicy.TableOf(m), HandSize: 7}
	}
	ask := decision.Decision{Kind: decision.KMulligan, Seq: 4, Min: 1, Max: 1,
		Prompt:  "p0 plays first. Keep your hand (put 0 cards on the bottom of your library) or take a mulligan?",
		Options: []decision.Option{{Index: 0, Kind: "keep"}, {Index: 1, Kind: "mulligan"}}}
	inner := &recBoard{recSeat{pick: "never"}}
	bs := Wrap(inner).(heuristic.BoardSeat)
	for lands, want := range map[int]int{0: 1, 1: 1, 2: 0, 3: 0, 5: 0, 6: 1, 7: 1} {
		in, err := bs.DecideBoard(context.Background(), hand(lands), ask)
		if err != nil || len(in.Choices) != 1 || in.Choices[0] != want || in.Seq != 4 {
			t.Errorf("%d lands: answer %+v err %v, want option %d", lands, in, err, want)
		}
	}
	if inner.got.Kind != "" {
		t.Fatal("the wrapped seat was consulted for a mulligan ask")
	}
}
