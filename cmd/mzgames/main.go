// Command mzgames plays two-seat games between gorge's default bot on the mage
// fork's decks (bots/mz/decks) and prints one line per game: the seed, the
// tap mode, the decks, turns, intents, the result and the event-chain head.
// gorge is deterministic, so the same flags print the same lines; mz-gorge
// uses that as its regression baseline for both autotap and manual tap
// (main_test.go pins a golden block).
//
// Usage:
//
//	mzgames -pair pauper-faeries,pauper-cawgates -games 8 -seed 1 -tap both
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/adams-shaun/gorge/bots"
	_ "github.com/adams-shaun/gorge/bots/all"
	"github.com/adams-shaun/gorge/bots/mz/decks"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/testkit/bench"
)

const (
	maxTurns   = 50 // the fork's max_turns
	maxIntents = 400000
)

func main() {
	pair := flag.String("pair", "pauper-faeries,pauper-cawgates", "two comma-separated deck names from bots/mz/decks")
	games := flag.Int("games", 8, "games per tap mode; game g uses seed+g")
	seed := flag.Uint64("seed", 1, "base seed")
	tap := flag.String("tap", "both", "auto | manual | both")
	dir := flag.String("cards", ".cards", "corpus directory")
	cost := flag.Int("cost", 0, "if > 0, instead of game lines print the clone/step/view cost row per tap mode, timing this many copies per root")
	suff := flag.Bool("suff", false, "instead of game lines, run the view-sufficiency probe (principle 1) per tap mode")
	gap := flag.Bool("gap", false, "instead of game lines, count castable casts autotap offers nothing for (principle 3)")
	atomicFlag := flag.Bool("atomic", false, "instead of game lines, play random games one atomic pick at a time (principle 2)")
	offer := flag.Bool("offer", false, "with -gap: switch on OfferPotentialCasts for both seats and play each extra cast on a clone")
	flag.Parse()

	reg, err := cards.SharedCorpus(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mzgames:", err)
		os.Exit(1)
	}
	names := strings.Split(*pair, ",")
	switch {
	case *gap:
		err = runGap(os.Stdout, reg, names, *games, *seed, *offer)
	case *atomicFlag:
		err = runAtomic(os.Stdout, reg, names, *games, *seed, *tap)
	default:
		err = run(os.Stdout, reg, names, *games, *seed, *tap, *cost, *suff)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mzgames:", err)
		os.Exit(1)
	}
}

// runGap measures the autotap gap on one pairing.
func runGap(out io.Writer, reg *cards.Registry, names []string, games int, seed uint64, offer bool) error {
	ds, err := loadPair(reg, names)
	if err != nil {
		return err
	}
	return measureGap(out, reg, names, ds, games, seed, offer)
}

func loadPair(reg *cards.Registry, names []string) ([][]*cards.Card, error) {
	if len(names) != 2 {
		return nil, fmt.Errorf("-pair needs two decks, got %d", len(names))
	}
	ds := make([][]*cards.Card, 2)
	for i, n := range names {
		d, err := decks.Load(reg, n)
		if err != nil {
			return nil, err
		}
		ds[i] = d
	}
	return ds, nil
}

// run plays games × tap modes on one goroutine, so its output order is fixed.
// With cost > 0 or suff it prints one cost or sufficiency row per mode instead
// of the game lines.
func run(out io.Writer, reg *cards.Registry, names []string, games int, seed uint64, tap string, cost int, suff bool) error {
	ds, err := loadPair(reg, names)
	if err != nil {
		return err
	}
	var modes []bool
	switch tap {
	case "auto":
		modes = []bool{true}
	case "manual":
		modes = []bool{false}
	case "both":
		modes = []bool{true, false}
	default:
		return fmt.Errorf("-tap %q: want auto, manual or both", tap)
	}
	for _, auto := range modes {
		if suff {
			if err := probeSufficiency(out, reg, names, ds, games, seed, auto); err != nil {
				return err
			}
			continue
		}
		if cost > 0 {
			if err := measureCost(out, reg, names, ds, games, seed, auto, cost); err != nil {
				return err
			}
			continue
		}
		for g := 0; g < games; g++ {
			line, err := playOne(reg, names, ds, seed+uint64(g), auto)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, line)
		}
	}
	return nil
}

// playOne plays one game and formats its line.
func playOne(reg *cards.Registry, names []string, ds [][]*cards.Card, seed uint64, auto bool) (string, error) {
	cfg := rules.Config{Seed: seed, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
	seats := make([]seat.Seat, 2)
	for i := range seats {
		s, err := bots.New(bots.Default, bots.Options{Seed: seed ^ uint64(i+1), AutoPayMana: auto})
		if err != nil {
			return "", err
		}
		seats[i] = s
	}
	o, e, err := bench.PlayGame(cfg, seats, maxTurns, maxIntents, bench.Hooks{})
	if err != nil {
		return "", fmt.Errorf("seed %d: %w", seed, err)
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
	return fmt.Sprintf("seed=%d tap=%s %s-vs-%s turns=%d intents=%d %s chain=%s",
		seed, mode, names[0], names[1], o.Turns, o.Intents, result, e.L.Head()), nil
}
