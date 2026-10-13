package main

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/rules"
)

// TestRandomIntentScratchKeepsTheStream pins the random row's per-seed
// outcome after W3 (action-walk.md §6): replacing math/rand/v2's Perm
// allocation with a reused in-place buffer must not move the RNG stream, so
// every seed still plays the identical game. The counts below were taken on
// base 2cb30d4bf (before W3) and must not move.
func TestRandomIntentScratchKeepsTheStream(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	a, err := testutil.LoadRepoDeck(reg, "mono-black-aggro")
	if err != nil {
		t.Fatal(err)
	}
	b, err := testutil.LoadRepoDeck(reg, "mono-red-prowess")
	if err != nil {
		t.Fatal(err)
	}
	// fixed seeds and a fixed game budget: the draws inside each game are
	// deterministic, so the totals are too. Re-pinned for the paid-cost mana
	// census default flip (2026-10-13; was 1600/38249, 1621/38800, 1648/40117):
	// the census offers more options, so the random games move. Forcing
	// rules/genesis.go's paidManaCensus to false restores the old totals.
	want := []struct {
		games, turns, decisions, rejected int
	}{
		{40, 1616, 38711, 0},
		{40, 1651, 39748, 0},
		{40, 1620, 39304, 0},
	}
	for i, w := range want {
		cfg := rules.Config{
			Seed:   77123 + uint64(i)*1000,
			Names:  []string{"a", "b"},
			Decks:  [][]*cards.Card{a, b},
			Tokens: reg.Tokens,
		}
		var st randomStats
		for g := 0; g < w.games; g++ {
			cfg.Seed = 77123 + uint64(i)*1000 + uint64(g)
			playRandom(cfg, &st)
		}
		if st.Games != w.games || st.Turns != w.turns || st.Decisions != w.decisions || st.Rejected != w.rejected {
			t.Errorf("seed set %d: games=%d turns=%d decisions=%d rejected=%d, want %d/%d/%d/%d",
				i, st.Games, st.Turns, st.Decisions, st.Rejected, w.games, w.turns, w.decisions, w.rejected)
		}
	}
}
