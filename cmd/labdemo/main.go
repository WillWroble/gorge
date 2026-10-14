// Command labdemo runs the draft-lab example workflow (E5-c4): play some
// random-deck games, freeze the player policy, build decks from sealed
// pools, draft 8-seat pods under Swiss, and gauntlet the built decks
// against a spellbench catalog — writing every 17Lands-shaped artifact to
// -out when it is set, and a summary to stdout either way. The whole
// workflow is a pure function of the flags: every step seeds its own
// math/rand/v2 PCG stream from -seed, so two runs with the same flags
// produce byte-identical artifacts.
//
//	labdemo -seed 1 -out /tmp/labdemo
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/adams-shaun/gorge/limited/setdata"
)

// options is one labdemo invocation, gathered from the flags.
type options struct {
	dir             string
	set             string
	collation       string
	seed            uint64
	randomGames     int
	player          string
	sealed          int
	pods            int
	podSeats        int
	opponents       string
	opponentCount   int
	opponentPlayers string
	gauntletHeroes  int
	games           int
	workers         int
	out             string
}

func main() {
	o := options{}
	fs := flag.NewFlagSet("labdemo", flag.ExitOnError)
	fs.StringVar(&o.dir, "dir", ".cards", "corpus directory (holds ir.gob.gz / cardsfolder)")
	fs.StringVar(&o.set, "set", "FDN", "set code the pools and pods are cut from")
	fs.StringVar(&o.collation, "collation", "", "booster collation; empty = the set's default")
	fs.Uint64Var(&o.seed, "seed", 1, "workflow seed; every step derives its own streams from it")
	fs.IntVar(&o.randomGames, "random-games", 4, "step 1: random-deck games to play")
	fs.StringVar(&o.player, "player", "bot@1", "player policy to freeze at step 2 (name or name@version)")
	fs.IntVar(&o.sealed, "sealed", 8, "step 3: sealed pools to build decks from")
	fs.IntVar(&o.pods, "pods", 1, "step 4: draft pods to run")
	fs.IntVar(&o.podSeats, "pod-seats", 8, "pod size: 4 or 8")
	fs.StringVar(&o.opponents, "opponents", "fdn-limited", "spellbench catalog the gauntlet opponents come from")
	fs.IntVar(&o.opponentCount, "opponent-count", 10, "opponents per gauntlet, from the catalog head (clamped to the catalog's size)")
	fs.StringVar(&o.opponentPlayers, "opponent-players", "bot,lethal-pressure", "comma-separated player policies, cycled over the opponents")
	fs.IntVar(&o.gauntletHeroes, "gauntlet-heroes", 0, "step 5: how many built decks lead a gauntlet (0 = every built deck)")
	fs.IntVar(&o.games, "games", 2, "games per gauntlet pairing")
	fs.IntVar(&o.workers, "workers", 4, "games run at once; never changes a result")
	fs.StringVar(&o.out, "out", "", "artifact directory; empty = the summary only")
	fs.Parse(os.Args[1:])

	if err := run(o, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "labdemo:", err)
		os.Exit(1)
	}
}

// playerList splits a comma-separated player flag into its names.
func playerList(s string) []string {
	return strings.Split(s, ",")
}

// collation is the run's collation value: the empty flag means setdata's
// default, which the lab resolves.
func collation(o options) setdata.Collation {
	return setdata.Collation(o.collation)
}
