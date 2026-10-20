package main

import (
	"fmt"
	"io"
	"time"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/mz"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/testkit/bench"
)

// runMZ plays the mz bot (seat 0) against gorge's bot (seat 1), offline,
// with a timeout long enough that the budget ends every search, so the
// games are deterministic. Per game it prints the game line and the search
// stats: sims per second and the cost of one search step.
func runMZ(out io.Writer, reg *cards.Registry, names []string, games int, seed uint64, tap string, budget int, prune bool) error {
	ds, err := loadPair(reg, names)
	if err != nil {
		return err
	}
	modes := map[string][]bool{"auto": {true}, "manual": {false}, "both": {true, false}}[tap]
	for _, auto := range modes {
		for g := 0; g < games; g++ {
			s := seed + uint64(g)
			opt := mz.Defaults()
			opt.Budget, opt.Timeout, opt.PruneDuplicates, opt.ManualTap = budget, time.Hour, prune, !auto
			bot := mz.New(opt)
			opp, err := bots.New(bots.Default, bots.Options{Seed: s ^ 2, AutoPayMana: auto})
			if err != nil {
				return err
			}
			cfg := rules.Config{Seed: s, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
			o, e, err := bench.PlayGame(cfg, []seat.Seat{bot, opp}, maxTurns, maxIntents, bench.Hooks{})
			if err != nil {
				return fmt.Errorf("seed %d: %w", s, err)
			}
			mode := "manual"
			if auto {
				mode = "auto"
			}
			result := "draw"
			switch {
			case o.IsStalled():
				result = "stall:" + o.StallOn
			case !o.Draw:
				result = "winner=" + names[o.WinnerSeat]
			}
			st := bot.Stats
			fmt.Fprintf(out, "mz seed=%d tap=%s %s-vs-%s turns=%d %s chain=%s searches=%d sims=%d sims_per_s=%.0f us_per_sim=%.0f reused=%d rebuilt=%d illegal=%d dup=%d replaced=%d\n",
				s, mode, names[0], names[1], o.Turns, result, e.L.Head(), st.Searches, st.Sims,
				float64(st.Sims)/st.Elapsed.Seconds(), float64(st.Elapsed.Microseconds())/float64(max(st.Sims, 1)),
				st.Reused, st.Rebuilt, st.Illegal, st.Duplicates, st.Replaced)
		}
	}
	return nil
}
