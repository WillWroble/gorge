// Command draftsim runs headless four-bot draft pods (draft-events spec
// §4.8, E0's exit gate): every seat drafts with limited/pickbot, builds its
// deck, and the pod's round-robin series are played as Bo1 games through the
// rules engine the way cmd/mtgsim drives one, with -verify replaying each
// event from its own log. It is the only E0 package that touches the
// engine; everything under limited/ and event/ stays pure.
//
//	draftsim -set FDN -seed 1 -events 5 -verify
//
// The event seed drives everything: the packs (booster.OpenPod), the picks
// (one seeded rand per seat) and the game seeds (derived per
// round/series/game), so the same seed and set rebuild the same event log
// byte for byte.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/adams-shaun/gorge/limited/setdata"
)

func main() {
	o := options{}
	fs := flag.NewFlagSet("draftsim", flag.ExitOnError)
	fs.StringVar(&o.set, "set", "FDN", "set code to draft (an embedded limited/setdata spec)")
	fs.StringVar(&o.collation, "collation", string(setdata.DefaultCollation), "collation the packs are cut from: "+string(setdata.Play)+", "+string(setdata.Arena))
	fs.Uint64Var(&o.seed, "seed", 1, "event seed; the first event uses it and each later event adds one")
	fs.IntVar(&o.events, "events", 1, "number of independent events to run")
	fs.StringVar(&o.players, "players", "lethal-pressure,bot,cast-profile,lethal-pressure",
		"comma-separated bot policy for seats 0..3 (a policy that reads decklists, bots.Info.Env, is refused)")
	fs.StringVar(&o.drafter, "drafter", "rating-colour", "pickbot drafter name")
	fs.StringVar(&o.builder, "builder", "best23", "pickbot builder name")
	fs.BoolVar(&o.verify, "verify", false, "replay each event from its own log and every game from its chain, exiting non-zero on the first mismatch")
	fs.StringVar(&o.draftlog, "draftlog", "", "path to append the events' picks to as 17Lands draft_data CSV")
	fs.StringVar(&o.gauntlet, "gauntlet", "", "deck-quality gauntlet after each event: play the built (and random baseline) decks against a spellbench catalog; only fdn-limited, and requires -set FDN")
	fs.IntVar(&o.gauntletGames, "gauntlet-games", 2, "games per gauntlet pairing, alternating which side sits in player 0's seat (one on the play's seat, one on the draw's)")
	fs.StringVar(&o.gauntletPlayer, "gauntlet-player", "bot", "bot policy for BOTH gauntlet seats (an Env policy is refused, like the draft seats)")
	fs.StringVar(&o.baseline, "baseline", "random", "gauntlet baseline deck per seat: random (23 random non-land pool cards + AutoLands) or none")
	fs.BoolVar(&o.verbose, "v", false, "print each seat's policy and deck summary")
	dir := fs.String("dir", ".cards", "corpus directory (holds ir.gob.gz / cardsfolder)")
	fs.Parse(os.Args[1:])
	o.dir = *dir

	out := os.Stdout
	if _, err := run(o, out); err != nil {
		fmt.Fprintln(os.Stderr, "draftsim:", err)
		os.Exit(1)
	}
}

// options is one draftsim invocation, gathered from the flags.
type options struct {
	set       string
	collation string
	seed      uint64
	events    int
	players   string
	drafter   string
	builder   string
	verify    bool
	draftlog  string
	verbose   bool
	dir       string
	// gauntlet, gauntletGames, gauntletPlayer and baseline are the
	// -gauntlet measurement flags (gauntlet.go); gauntletMaxHuman caps the
	// human deck list for tests and is not a flag.
	gauntlet         string
	gauntletGames    int
	gauntletPlayer   string
	baseline         string
	gauntletMaxHuman int
}

// playerList splits the -players flag into its four seat policies.
func (o options) playerList() ([]string, error) {
	players := strings.Split(o.players, ",")
	for i := range players {
		players[i] = strings.TrimSpace(players[i])
	}
	if len(players) != 4 {
		return nil, fmt.Errorf("-players names %d seats, want 4 (got %q)", len(players), o.players)
	}
	return players, nil
}
