package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// batchRecorder wraps the fakeHost double to keep every BatchDepartures call
// the primitive makes: the double's EndBatchDepartures clears its single
// batch field, so a caller observing after the effect returns cannot tell a
// batch from no batch.
type batchRecorder struct {
	*fakeHost
	calls [][]state.ObjID
	open  bool
}

func (b *batchRecorder) BatchDepartures(ids []state.ObjID) {
	b.calls = append(b.calls, ids)
	b.open = true
	b.fakeHost.BatchDepartures(ids)
}

func (b *batchRecorder) EndBatchDepartures() {
	b.open = false
	b.fakeHost.EndBatchDepartures()
}

// The plain Defined$ object targets of one effSacrifice call sacrifice as
// ONE simultaneous departure batch (CR 614.6/616.1 with 603.10): a departing
// replacement source among them (Kalitas, Traitor of Ghet) keeps its
// ActiveZones$ Battlefield replacement for the later members, so the batch
// must cover all of them before the first Sacrifice emits. No corpus card
// reaches this shape with more than one object target today (every multi-
// permanent Sacrifice line names players), so the wrap is pinned at the unit
// level with the recorder's call history. The rules-side application of an
// open batch is pinned by the oracle scenarios in
// rules/masszone_rest_repl_test.go.
func TestSacrificeDefinedTargetsBatch(t *testing.T) {
	h := &batchRecorder{fakeHost: newHost(t, 2)}
	kalitas := putBattlefield(h.fakeHost, 0, "Name:Kalitas\nManaCost:2 B B\nTypes:Creature Vampire\nPT:3/4\nOracle:x\n")
	victim := putBattlefield(h.fakeHost, 1, "Name:Victim\nManaCost:1 G\nTypes:Creature Beast\nPT:2/2\nOracle:x\n")
	// Preconditions the assertion depends on: both victims on the battlefield
	// and no batch call yet.
	if h.g.Obj(kalitas).Zone != state.ZBattlefield || h.g.Obj(victim).Zone != state.ZBattlefield {
		t.Fatalf("setup: victims not both on the battlefield (%v %v)",
			h.g.Obj(kalitas).Zone, h.g.Obj(victim).Zone)
	}
	if len(h.calls) != 0 {
		t.Fatalf("setup: batch already called: %v", h.calls)
	}

	c := &Ctx{Source: 1, Controller: 0, Targets: []state.Target{{Obj: kalitas}, {Obj: victim}}, TargetsOffered: true}
	effSacrifice(h, c, sacrificeParams(map[string]string{"Defined": "Targeted", "SacValid": "Creature"}))

	if len(h.calls) != 1 {
		t.Fatalf("BatchDepartures calls = %v, want exactly one covering both victims", h.calls)
	}
	call := h.calls[0]
	if len(call) != 2 || call[0] != kalitas || call[1] != victim {
		t.Fatalf("BatchDepartures = %v, want both victims [%d %d]", call, kalitas, victim)
	}
	if h.open {
		t.Fatalf("the batch was never closed")
	}
	if sacZone(h.fakeHost, kalitas) != state.ZGraveyard {
		t.Fatalf("Kalitas zone = %v, want graveyard", sacZone(h.fakeHost, kalitas))
	}
	if sacZone(h.fakeHost, victim) != state.ZGraveyard {
		t.Fatalf("victim zone = %v, want graveyard", sacZone(h.fakeHost, victim))
	}
}
