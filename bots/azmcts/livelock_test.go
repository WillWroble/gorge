package azmcts

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
)

// TestSearchScoresAWorldLivelockAsAStallDraw: a world that trips the
// engine's livelock contract (*rules.LivelockError -- here the armed object
// cap; in the G0 hands-bot gate, seed 15484239965368256534 seq 1910, the
// token-plan mint cap: an Extravagant Replication copying a token doubler
// every upkeep doubles the plan past 2^20) is an unrepresentable game the
// bench ends as a stall. The walk scores it a terminal draw and counts it
// Livelocks; it is never a recovered Panic (a discarded simulation).
func TestSearchScoresAWorldLivelockAsAStallDraw(t *testing.T) {
	cfg := testConfig(t, "foundations-calling-all-angels", "foundations-wretched-ranks", testSeed)
	e0, _, _ := botPosition(t, cfg, decision.KPriority, 3, 4000)
	// Arm the cap one object above the position's arena: the replay to the
	// position is unchanged, and the worlds trip as soon as they grow it.
	cfg.LoopGuard = &rules.LoopGuard{MaxObjs: len(e0.G.Objs)}
	e, d, bot := botPosition(t, cfg, decision.KPriority, 3, 4000)
	if e.L.Head() != e0.L.Head() {
		t.Fatal("the armed cap changed the replay to the position")
	}
	opts := DefaultOptions()
	opts.Sims, opts.MaxSteps = 64, 2000
	res := searchAt(t, e, d, bot, nil, opts)
	st := res.Stats
	t.Logf("stats: sims %d completed %d livelocks %d panics %d terminal %d", st.Simulations, st.Completed, st.Livelocks, st.Panics, st.Terminal)
	if st.Livelocks == 0 && st.Panics == 0 {
		t.Fatal("no world tripped the armed cap: the test measures nothing")
	}
	if st.Panics != 0 || st.Livelocks == 0 || st.Completed != st.Simulations {
		t.Fatalf("panics %d, livelocks %d, completed %d of %d: a livelocked world must be a scored stall leaf, not a discarded panic", st.Panics, st.Livelocks, st.Completed, st.Simulations)
	}
}
