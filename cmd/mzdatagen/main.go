// Command mzdatagen is MageZero's data generator on gorge: the mage fork's
// MageZeroMain and ParallelDataGenerator. It reads the fork's config.yml,
// plays its games on a pool of workers, labels each player's searched states
// by the game's result (td_discount), and writes them per player as
// data/playerA/<A>_vs_<B>.npz and data/playerB/<B>_vs_<A>.npz (bots/mz/data's
// layout; the fork writes the same datasets as HDF5).
//
// Unlike the fork, game g plays with seed+g, and rows are written in game
// order, so the same config, seed and model answers give the same files.
//
// Usage:
//
//	mzdatagen [-seed 1] [-cards .cards] [config.yml]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adams-shaun/gorge/bots"
	_ "github.com/adams-shaun/gorge/bots/all"
	"github.com/adams-shaun/gorge/bots/mz"
	"github.com/adams-shaun/gorge/bots/mz/data"
	"github.com/adams-shaun/gorge/bots/mz/decks"
	"github.com/adams-shaun/gorge/bots/mz/remote"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/testkit/bench"
)

const (
	maxIntents = 400000
	mulligans  = 7 // the fork's London mulligan round: each player decides
)

func main() {
	seed := flag.Uint64("seed", 1, "base seed: game g uses seed+g")
	dir := flag.String("cards", ".cards", "corpus directory")
	flag.Parse()
	path := "config/config.yml"
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}
	if err := run(os.Stdout, path, *dir, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "mzdatagen:", err)
		os.Exit(1)
	}
}

func run(out io.Writer, path, dir string, seed uint64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	cfg, err := parseConfig(f)
	f.Close()
	if err != nil {
		return err
	}
	if cfg.GameMode != "normal" {
		return fmt.Errorf("game_mode %q is not supported", cfg.GameMode)
	}
	reg, err := cards.SharedCorpus(dir)
	if err != nil {
		return err
	}
	g, err := newGenerator(out, cfg, reg)
	if err != nil {
		return err
	}
	return g.generate(seed)
}

type generator struct {
	out     io.Writer
	cfg     Config
	reg     *cards.Registry
	names   [2]string // the decks' names, as extractDeckName gives them
	decks   [2][]*cards.Card
	players [2]PlayerConfig
	nets    [2]*remote.Client
}

func newGenerator(out io.Writer, cfg Config, reg *cards.Registry) (*generator, error) {
	g := &generator{out: out, cfg: cfg, reg: reg, players: [2]PlayerConfig{cfg.PlayerA, cfg.PlayerB}}
	ports := [2]int{cfg.Server.Port, cfg.Server.OpponentPort}
	for i, p := range g.players {
		g.names[i] = extractDeckName(p.DeckPath)
		d, err := decks.Load(reg, strings.ToLower(g.names[i]))
		if err != nil {
			return nil, err
		}
		g.decks[i] = d
		if p.Type == "mcts" && !p.OfflineMode {
			url := fmt.Sprintf("http://%s:%d", cfg.Server.Host, ports[i])
			if g.nets[i], err = remote.Dial(url); err != nil {
				fmt.Fprintf(out, "failed to establish connection to network model %c at %s; falling back to offline mode\n", 'A'+i, url)
			}
		}
	}
	return g, nil
}

// extractDeckName is the deck file's base name without its extension.
func extractDeckName(path string) string {
	base := filepath.Base(strings.ReplaceAll(path, `\`, "/"))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// result is one game's: each player's labeled rows, or a failure.
type result struct {
	rows  [2][]mz.Row
	aWon  bool
	stall string
	err   error
}

func (g *generator) generate(seed uint64) error {
	files := [2]string{
		filepath.Join("data", "playerA", g.names[0]+"_vs_"+g.names[1]+".npz"),
		filepath.Join("data", "playerB", g.names[1]+"_vs_"+g.names[0]+".npz"),
	}
	var ws [2]*data.Writer
	for i, p := range g.players {
		if p.OutputFile != "" {
			files[i] = strings.TrimSuffix(p.OutputFile, ".hdf5")
			if filepath.Ext(files[i]) != ".npz" {
				files[i] += ".npz"
			}
		}
		w, err := data.Create(files[i])
		if err != nil {
			return err
		}
		ws[i] = w
	}

	n := g.cfg.Training.Games
	fmt.Fprintf(g.out, "simulating %d games on %d workers\n", n, g.cfg.Training.Threads)
	results := make([]chan result, n)
	for i := range results {
		results[i] = make(chan result, 1)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range max(1, g.cfg.Training.Threads) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] <- g.play(i, seed+uint64(i))
			}
		}()
	}
	go func() {
		for i := range n {
			jobs <- i
		}
		close(jobs)
	}()

	wins, ok, failed := 0, 0, 0
	for i := range n {
		r := <-results[i]
		switch {
		case r.err != nil:
			failed++
			fmt.Fprintf(g.out, "game %d failed and is ignored: %v\n", i+1, r.err)
			continue
		case r.stall != "":
			failed++
			fmt.Fprintf(g.out, "game %d stopped (%s) and is ignored\n", i+1, r.stall)
			continue
		}
		ok++
		if r.aWon {
			wins++
		}
		for p, w := range ws {
			for _, row := range r.rows[p] {
				if err := w.Write(row); err != nil {
					return err
				}
			}
			w.EndGame()
		}
		if g.cfg.Logging.ShowWR {
			fmt.Fprintf(g.out, "game %d completed: player A %s; current WR %.3f\n", i+1, map[bool]string{true: "won", false: "lost"}[r.aWon], float64(wins)/float64(ok))
		}
	}
	wg.Wait()
	for i, w := range ws {
		fmt.Fprintf(g.out, "player %c: %d states to %s\n", 'A'+i, w.Rows(), files[i])
		if err := w.Close(); err != nil {
			return err
		}
	}
	wr := 0.0
	if ok > 0 {
		wr = float64(wins) / float64(ok)
	}
	fmt.Fprintf(g.out, "requested %d, successful %d, failed %d; player A win rate %.2f%% (%d/%d)\n", n, ok, failed, 100*wr, wins, ok)
	if g.cfg.Logging.WriteFinalWR {
		f, err := os.OpenFile("WinRates.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		fmt.Fprintf(f, "WR with %s vs %s: %v in %d games\n", g.names[0], g.names[1], wr, ok)
		f.Close()
	}
	return nil
}

// play is runSingleGame: PlayerA and PlayerB, the starting player by
// goes_first (random alternates by game), a game cut at max_turns (counted:
// player A did not win) or dropped at max_minutes, and the rows labeled.
func (g *generator) play(i int, seed uint64) result {
	var seats [2]seat.Seat
	var mzs [2]*mz.Bot
	for p, pc := range g.players {
		if pc.Type == "minimax" {
			s, err := bots.New(bots.Default, bots.Options{Seed: seed ^ uint64(p+1), AutoPayMana: true})
			if err != nil {
				return result{err: err}
			}
			seats[p] = s
			continue
		}
		mzs[p] = mz.New(options(pc, seed^uint64(p+1)), g.nets[p])
		seats[p] = mzs[p]
	}
	starter := i % 2
	switch g.cfg.GoesFirst {
	case "player_a":
		starter = 0
	case "player_b":
		starter = 1
	}
	cfg := rules.Config{
		Seed: seed, Names: g.names[:], PlayerNames: []string{"PlayerA", "PlayerB"},
		Decks: g.decks[:], Tokens: g.reg.Tokens, NameUniverse: g.reg.Universe(),
		Mulligans: mulligans, StartChooser: &starter,
	}
	cfg.LoopGuard = rules.WithDefaultObjectCap(cfg.LoopGuard)
	deadline := time.Now().Add(time.Duration(g.cfg.Training.MaxMinutes) * time.Minute)
	hooks := bench.Hooks{Guard: func(*rules.Engine) (string, string) {
		if time.Now().After(deadline) {
			return "timeout", ""
		}
		return "", ""
	}}
	o, _, err := bench.PlayGame(cfg, seats[:], g.cfg.Training.MaxTurns, maxIntents, hooks)
	if err != nil {
		return result{err: err}
	}
	if o.IsStalled() && o.StallOn != "turns" {
		return result{stall: o.StallOn}
	}
	r := result{aWon: !o.IsStalled() && !o.Draw && o.WinnerSeat == 0}
	tds := [2]float64{g.players[0].TDDiscount, g.players[1].TDDiscount}
	for p, b := range mzs {
		if b != nil {
			mz.Label(b.Rows, r.aWon == (p == 0), tds[p])
			r.rows[p] = b.Rows
		}
	}
	return r
}

// options is configurePlayer for an MCTS player.
func options(pc PlayerConfig, seed uint64) mz.Options {
	o := mz.Defaults()
	o.Budget, o.Timeout = pc.SearchBudget, time.Duration(pc.TimeoutMs)*time.Millisecond
	o.BackpropDiscount, o.PruneDuplicates = pc.BackpropDiscount, pc.PruneDuplicateStates
	o.ManualTap, o.Mulligans, o.SeeOpponentHand, o.Offline = pc.ManualTap, pc.Mulligans, pc.OpponentHand, pc.OfflineMode
	o.PriorTemperature, o.PriorBonus = pc.PriorTemperature, pc.PriorBonus
	o.NoPolicyPriority, o.NoPolicyTarget, o.NoPolicyUse, o.NoPolicyOpponent = !pc.Priority, !pc.Target, !pc.Binary, !pc.Opponent
	o.NoNoise, o.DirichletNoiseEps, o.SelectionTemperature = !pc.NoiseEnabled, pc.DirichletNoise, pc.SelectionTemperature
	o.Seed = seed
	return o
}
