package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// observeUntilTrip feeds distinct (aperiodic) Note events at the given arena
// population until the watcher panics, returning the abort (nil if none).
func observeUntilTrip(w *livelockWatcher, n, nObjs int, mk func(i int) events.Event) (lle *LivelockError) {
	defer func() {
		if r := recover(); r != nil {
			l, ok := r.(*LivelockError)
			if !ok {
				panic(r)
			}
			lle = l
		}
	}()
	for i := 0; i < n; i++ {
		ev := mk(i)
		ev.Seq = uint64(i + 1)
		w.observeFrom(&ev, 0, nObjs)
	}
	return nil
}

func distinctNote(i int) events.Event {
	return events.Event{Kind: events.Note, Obj: state.ObjID(i + 1)}
}

// A runaway burst on a board of RunawayBigBoardObjs or more is the
// big-board reason (the harness stalls it); the same burst on a small board
// stays the engine-bug "runaway resolution"; a real repeating cycle on a big
// board still aborts as a cycle.
func TestLivelockRunawayBigBoardReasonDirections(t *testing.T) {
	t.Parallel()
	g := &LoopGuard{RunawayEvents: 50, CycleEvents: 30, MaxPeriod: 8}

	small := observeUntilTrip(ptrW(g), 200, RunawayBigBoardObjs-1, distinctNote)
	if small == nil || small.Reason != "runaway resolution" {
		t.Fatalf("small board: got %+v, want reason runaway resolution", small)
	}

	big := observeUntilTrip(ptrW(g), 200, RunawayBigBoardObjs, distinctNote)
	if big == nil || big.Reason != RunawayBigBoardReason || big.Count != RunawayBigBoardObjs {
		t.Fatalf("big board: got %+v, want reason %q count %d", big, RunawayBigBoardReason, RunawayBigBoardObjs)
	}
	if big.Error() == "" || livelockReasonCodes.Code(big.Reason) != livelockReasonBigBoard {
		t.Fatalf("big-board reason not coded: %q", big.Error())
	}

	cycle := observeUntilTrip(ptrW(g), 200, RunawayBigBoardObjs*3, func(int) events.Event {
		return events.Event{Kind: events.Note, Obj: 42}
	})
	if cycle == nil || cycle.Reason != "repeating cycle" {
		t.Fatalf("real cycle on a big board: got %+v, want repeating cycle", cycle)
	}
}

func ptrW(g *LoopGuard) *livelockWatcher {
	w := newLivelockWatcher(g)
	return &w
}
