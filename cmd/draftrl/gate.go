package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/internal/memcap"
	"github.com/adams-shaun/gorge/limited/pickbot"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// gateSalt tags each measured deck's gauntlet seed stream: (event seed,
// kind, seat) -> one Lab.Gauntlet seed.
const gateSalt = 0xD4A7E5

// The measured deck kinds.
const (
	kindBuilt = iota
	kindRandom
)

// gateOpts is the parsed gate configuration.
type gateOpts struct {
	corpus, set, collation string
	seed                   uint64
	events, games          int
	drafters, builders     []seatAgent
	baseline               string
	pilot                  string
	azNet                  string
	azSims, azBelief       int
	azBeliefDir            string
	azBeliefPool           string
	azPanics               int
	onlyGame               uint64
	onlyGameTape           string
	workers, maxObjs       int
	maxHuman               int
	out                    string
}

// seatAgent is one seat's drafter or builder spec: "17l" (the embedded
// 17Lands ratings), "ovr:PATH#FIELD" (a ratings override file and field,
// sim/training only), or for builders "random" (draftsim's random deck
// from the seat's own pool).
type seatAgent struct {
	kind  string // "17l", "ovr", "random"
	path  string
	field string
}

func (a seatAgent) String() string {
	switch a.kind {
	case "ovr":
		return "ovr:" + filepath.Base(a.path) + "#" + a.field
	case "17l", "random":
	default:
		return a.kind + ":" + filepath.Base(a.path)
	}
	return a.kind
}

func parseSeatAgents(s string, builder bool) ([]seatAgent, error) {
	var out []seatAgent
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		switch {
		case tok == "17l":
			out = append(out, seatAgent{kind: "17l"})
		case builder && tok == "random":
			out = append(out, seatAgent{kind: "random"})
		case strings.HasPrefix(tok, "ovr:"):
			path, field, ok := strings.Cut(strings.TrimPrefix(tok, "ovr:"), "#")
			if !ok || path == "" || field == "" {
				return nil, fmt.Errorf("seat agent %q: want ovr:PATH#FIELD", tok)
			}
			out = append(out, seatAgent{kind: "ovr", path: path, field: field})
		case !builder && strings.Contains(tok, ":"):
			// A registered league DrafterKind, e.g. net:WEIGHTS.dpkw.
			kind, arg, _ := strings.Cut(tok, ":")
			if _, ok := lookupDrafterKind(kind); !ok {
				return nil, fmt.Errorf("seat agent %q: no drafter kind %q is linked into this build", tok, kind)
			}
			out = append(out, seatAgent{kind: kind, path: arg})
		default:
			return nil, fmt.Errorf("unknown seat agent %q (want 17l, ovr:PATH#FIELD%s)", tok, map[bool]string{true: " or random", false: ""}[builder])
		}
	}
	if len(out) != 1 && len(out) != 4 {
		return nil, fmt.Errorf("%d seat agents, want 1 or 4", len(out))
	}
	return out, nil
}

func parseGate(args []string) (gateOpts, error) {
	var o gateOpts
	var drafters, builders string
	fs := flag.NewFlagSet("draftrl gate", flag.ContinueOnError)
	fs.StringVar(&o.corpus, "corpus", ".cards", "card corpus directory")
	fs.StringVar(&o.set, "set", "FDN", "set code")
	fs.StringVar(&o.collation, "collation", string(setdata.DefaultCollation), "booster collation")
	fs.Uint64Var(&o.seed, "seed", 201, "first event seed; event n uses seed+n")
	fs.IntVar(&o.events, "events", 24, "events (4-seat pods)")
	fs.IntVar(&o.games, "games", 4, "games per gauntlet pairing (sides alternate)")
	fs.StringVar(&drafters, "drafters", "17l", "per-seat drafter specs, 1 or 4, comma-separated: 17l | ovr:PATH#FIELD (rating-colour) | KIND:ARG (a registered DrafterKind, e.g. net:WEIGHTS.dpkw[@TABLE.json][~T], ~T sampling at temperature T)")
	fs.StringVar(&builders, "builders", "", "per-seat builder specs, 1 or 4: 17l | ovr:PATH#FIELD (best23) | random; empty = follow each seat's drafter ratings")
	fs.StringVar(&o.baseline, "baseline", "random", "also measure a random deck from each seat's pool: random | none")
	fs.StringVar(&o.pilot, "pilot", "bot", "gauntlet pilot on both seats: a bot policy name, or az (closed-list az search)")
	fs.StringVar(&o.azNet, "az-net", "", "az: the .dzgw network; empty = no network")
	fs.IntVar(&o.azSims, "az-sims", 25, "az: simulations per searched decision")
	fs.IntVar(&o.azBelief, "az-belief", 500, "az: random decks (from training-seed sealed pools) in the closed-list belief, when -az-belief-dir is empty")
	fs.StringVar(&o.azBeliefDir, "az-belief-dir", "", "az: deck directory for searchprobe.LoadBeliefDir")
	fs.StringVar(&o.azBeliefPool, "az-belief-pool", "", "az: pool.tsv naming the train split of -az-belief-dir")
	fs.IntVar(&o.azPanics, "az-panics", 3, "az: print the first N distinct recovered search-panic stacks (and every panic's game seed) to stderr; 0 = off")
	fs.Uint64Var(&o.onlyGame, "only-game", 0, "debug: play only the gauntlet game with this engine seed (as printed by -az-panics / the fallback trace) and print where it sits; 0 = the whole gate")
	fs.StringVar(&o.onlyGameTape, "only-game-tape", "", "debug: with -only-game, write the game's tape (seed, names, deck lists, intents) as JSON to this path, for a capped regression test")
	fs.IntVar(&o.workers, "workers", 4, "concurrent gauntlets")
	fs.IntVar(&o.maxObjs, "max-objs", 2000, "engine board-size cap (0 = off)")
	fs.IntVar(&o.maxHuman, "max-human", 0, "cap the gauntlet's human decks (tests); 0 = all")
	fs.StringVar(&o.out, "out", "", "optional output directory (gate.json, az.json)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	var err error
	if o.drafters, err = parseSeatAgents(drafters, false); err != nil {
		return o, fmt.Errorf("-drafters: %w", err)
	}
	if builders == "" {
		o.builders = o.drafters
		for _, d := range o.drafters {
			if d.kind != "17l" && d.kind != "ovr" {
				return o, fmt.Errorf("-drafters %s has no ratings to build with: give -builders", d)
			}
		}
	} else if o.builders, err = parseSeatAgents(builders, true); err != nil {
		return o, fmt.Errorf("-builders: %w", err)
	}
	switch {
	case o.events < 1:
		return o, fmt.Errorf("-events must be at least 1")
	case o.games < 1:
		return o, fmt.Errorf("-games must be at least 1")
	case o.baseline != "random" && o.baseline != "none":
		return o, fmt.Errorf("unknown -baseline %q", o.baseline)
	case o.workers < 1:
		return o, fmt.Errorf("-workers must be at least 1")
	}
	return o, nil
}

// at picks seat s's entry from a 1-or-4 list.
func at(xs []seatAgent, s int) seatAgent {
	if len(xs) == 1 {
		return xs[0]
	}
	return xs[s]
}

// gateResult is what one gate run measured, for the tests and gate.json.
type gateResult struct {
	Built, Random []draftlab.Tally // per seat
	BuiltPooled   draftlab.Tally
	RandomPooled  draftlab.Tally
	Games         int
	AZ            *azCount `json:",omitempty"`
}

// gate runs the gate subcommand.
func gate(args []string, out io.Writer) (*gateResult, error) {
	o, err := parseGate(args)
	if err != nil {
		return nil, err
	}
	az := o.pilot == "az"
	if az {
		// Refuse an az run whose memory settings cannot hold its working set,
		// before the corpus loads, exactly as cardval does. See internal/memcap.
		if err := memcap.CheckAzCaps(o.workers, o.azSims); err != nil {
			return nil, err
		}
	}
	lab, err := draftlab.Open(o.corpus, draftlab.Options{AllowDecklistBots: az, MaxObjs: o.maxObjs, Workers: 1, RecordTape: o.onlyGameTape != ""})
	if err != nil {
		return nil, err
	}
	coll := setdata.Collation(o.collation)
	overrides := map[string]*pickbot.RatingsOverride{}
	loadOvr := func(a seatAgent) (*pickbot.RatingsOverride, error) {
		if a.kind != "ovr" {
			return nil, nil
		}
		key := a.path + "#" + a.field
		if ov, ok := overrides[key]; ok {
			return ov, nil
		}
		b, err := os.ReadFile(a.path)
		if err != nil {
			return nil, err
		}
		ov, err := pickbot.ParseRatingsOverride(b, a.path, a.field, o.set)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(out, "ratings override: %s field %s; %d 17Lands-rated cards missing (neutral)\n", a.path, a.field, len(ov.Missing))
		overrides[key] = ov
		return ov, nil
	}

	pilotRef := o.pilot
	var stats *azCount
	if az {
		var label string
		if stats, label, err = registerAZ(lab, o); err != nil {
			return nil, err
		}
		pilotRef = azPolicy
		fmt.Fprintf(out, "pilot: %s\n", label)
	}
	ref, err := draftlab.ParseAgent(pilotRef)
	if err != nil {
		return nil, err
	}
	player, err := lab.Player(ref)
	if err != nil {
		return nil, err
	}
	catalog, err := lab.CatalogDecks("fdn-limited")
	if err != nil {
		return nil, err
	}
	if o.maxHuman > 0 && o.maxHuman < len(catalog) {
		catalog = catalog[:o.maxHuman]
	}
	opponents := make([]draftlab.Entrant, len(catalog))
	for i, d := range catalog {
		opponents[i] = draftlab.Entrant{Deck: d, Player: player}
	}
	ds := make([]string, 4)
	bs := make([]string, 4)
	for s := 0; s < 4; s++ {
		ds[s], bs[s] = at(o.drafters, s).String(), at(o.builders, s).String()
	}
	fmt.Fprintf(out, "gate: set %s (%s), seeds %d-%d, drafters %s, builders %s, pilot %s, %d human decks x %d games\n",
		o.set, coll, o.seed, o.seed+uint64(o.events)-1, strings.Join(ds, ","), strings.Join(bs, ","), player.Ref, len(opponents), o.games)

	// Phase 1: draft and build every event serially (cheap).
	type job struct {
		ev, seat, kind int
		seed           uint64
		deck           draftlab.Deck
	}
	var jobs []job
	for n := 0; n < o.events; n++ {
		seed := o.seed + uint64(n)
		packs, err := lab.OpenPod(o.set, coll, seed, 4, 3)
		if err != nil {
			return nil, fmt.Errorf("event seed %d: %w", seed, err)
		}
		drafters := make([]draftlab.NamedDrafter, 4)
		for s := range drafters {
			a := at(o.drafters, s)
			if k, ok := lookupDrafterKind(a.kind); ok && a.kind != "17l" && a.kind != "ovr" {
				kd, kref, err := k(lab, a.path, o.set)
				if err != nil {
					return nil, fmt.Errorf("-drafters %s: %w", a, err)
				}
				drafters[s] = draftlab.NamedDrafter{Ref: kref, Drafter: kd}
				continue
			}
			d, err := lab.Drafter(draftlab.AgentRef{Name: "rating-colour"})
			if err != nil {
				return nil, err
			}
			ov, err := loadOvr(a)
			if err != nil {
				return nil, err
			}
			if ov != nil {
				d.Drafter.(*pickbot.RatingColour).Ratings = ov
			}
			drafters[s] = d
		}
		dr, err := lab.Draft(packs, drafters, seed)
		if err != nil {
			return nil, fmt.Errorf("event seed %d: %w", seed, err)
		}
		for s, pool := range dr.Pools {
			a := at(o.builders, s)
			var deck draftlab.Deck
			if a.kind == "random" {
				deck, err = lab.RandomDeck(pool, gauntletSeed(seed, kindBuilt, s))
			} else {
				var b draftlab.NamedBuilder
				if b, err = lab.Builder(draftlab.AgentRef{Name: "best23"}); err != nil {
					return nil, err
				}
				ov, err := loadOvr(a)
				if err != nil {
					return nil, err
				}
				if ov != nil {
					b.Builder.(*pickbot.Best23).Ratings = ov
				}
				deck, err = lab.Build(pool, b)
			}
			if err != nil {
				return nil, fmt.Errorf("event seed %d seat %d: %w", seed, s, err)
			}
			deck.Name = fmt.Sprintf("e%d-s%d", seed, s)
			jobs = append(jobs, job{ev: n, seat: s, kind: kindBuilt, seed: gauntletSeed(seed, kindBuilt, s), deck: deck})
			if o.baseline == "random" {
				rd, err := lab.RandomDeck(pool, gauntletSeed(seed, kindRandom, s))
				if err != nil {
					return nil, err
				}
				rd.Name = fmt.Sprintf("e%d-s%d-random", seed, s)
				jobs = append(jobs, job{ev: n, seat: s, kind: kindRandom, seed: gauntletSeed(seed, kindRandom, s), deck: rd})
			}
		}
	}

	if o.onlyGame != 0 {
		for _, j := range jobs {
			for oi := range opponents {
				for g := 0; g < o.games; g++ {
					if draftlab.GauntletGameSeed(j.seed, oi, g) != o.onlyGame {
						continue
					}
					fmt.Fprintf(out, "only-game %d: hero %s vs %s, game %d\n", o.onlyGame, j.deck.Name, opponents[oi].Deck.Name, g)
					res, err := lab.GauntletGame(draftlab.Entrant{Deck: j.deck, Player: player}, opponents[oi], g, o.onlyGame)
					if err != nil {
						return nil, err
					}
					fmt.Fprintf(out, "only-game: winner %d turns %d intents %d head %x\n", res.Winner, res.Turns, res.Intents, res.Head)
					if o.onlyGameTape != "" && res.Tape != nil {
						b, err := json.MarshalIndent(struct {
							Seed      uint64            `json:"seed"`
							Names     [2]string         `json:"names"`
							Mulligans int               `json:"mulligans"`
							MaxObjs   int               `json:"max_objs"`
							Decks     [2][]string       `json:"decks"`
							Intents   []decision.Intent `json:"intents"`
						}{o.onlyGame, res.Tape.Names, res.Tape.Mulligans, o.maxObjs, seatLists(j.deck, opponents[oi].Deck, g), res.Tape.Intents}, "", " ")
						if err != nil {
							return nil, err
						}
						if err := os.WriteFile(o.onlyGameTape, b, 0o644); err != nil {
							return nil, err
						}
					}
					if stats != nil {
						fmt.Fprintf(out, "  az: %s\n", stats.summary())
					}
					return &gateResult{AZ: stats}, nil
				}
			}
		}
		return nil, fmt.Errorf("-only-game %d: no gauntlet game has this seed", o.onlyGame)
	}

	// Phase 2: every gauntlet, -workers at once, results in job order.
	tallies := make([]draftlab.Tally, len(jobs))
	errs := make([]error, len(jobs))
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < o.workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				j := jobs[i]
				res, err := lab.Gauntlet(draftlab.Entrant{Deck: j.deck, Player: player}, opponents, o.games, j.seed)
				if err != nil {
					errs[i] = fmt.Errorf("%s: %w", j.deck.Name, err)
					continue
				}
				tallies[i] = res.Pooled
			}
		}()
	}
	for i := range jobs {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	r := &gateResult{Built: make([]draftlab.Tally, 4), Random: make([]draftlab.Tally, 4), AZ: stats}
	for i, j := range jobs {
		t := &r.Built[j.seat]
		if j.kind == kindRandom {
			t = &r.Random[j.seat]
		}
		addTally(t, tallies[i])
		r.Games += tallies[i].Games
	}
	for s := 0; s < 4; s++ {
		printTally(out, fmt.Sprintf("built seat %d:", s), &r.Built[s])
		addTally(&r.BuiltPooled, r.Built[s])
	}
	printTally(out, "built:", &r.BuiltPooled)
	if o.baseline == "random" {
		for s := 0; s < 4; s++ {
			printTally(out, fmt.Sprintf("random seat %d:", s), &r.Random[s])
			addTally(&r.RandomPooled, r.Random[s])
		}
		printTally(out, "random:", &r.RandomPooled)
	}
	if stats != nil {
		fmt.Fprintf(out, "  az: %s\n", stats.summary())
	}
	if o.out != "" {
		if err := os.MkdirAll(o.out, 0o755); err != nil {
			return nil, err
		}
		b, _ := json.MarshalIndent(r, "", " ")
		if err := os.WriteFile(filepath.Join(o.out, "gate.json"), b, 0o644); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// gauntletSeed derives one measured deck's Lab.Gauntlet seed.
func gauntletSeed(seed uint64, kind, s int) uint64 {
	return rand.New(rand.NewPCG(seed, gateSalt<<32|uint64(kind)<<8|uint64(s))).Uint64()
}

func addTally(dst *draftlab.Tally, t draftlab.Tally) {
	dst.Wins += t.Wins
	dst.Losses += t.Losses
	dst.Draws += t.Draws
	dst.Games += t.Games
	dst.WinRate, dst.Lo95, dst.Hi95 = wilsonCI(dst.Wins, dst.Games)
}

// printTally writes one line in cmd/draftsim's contract format:
// "LABEL W-L-D  win% P  ci95 [LO, HI]".
func printTally(out io.Writer, label string, t *draftlab.Tally) {
	t.WinRate, t.Lo95, t.Hi95 = wilsonCI(t.Wins, t.Games)
	fmt.Fprintf(out, "  %-16s %d-%d-%d  win%% %.3f  ci95 [%.3f, %.3f]\n",
		label, t.Wins, t.Losses, t.Draws, t.WinRate, t.Lo95, t.Hi95)
}

// wilsonCI is cmd/draftsim's Wilson score 95% interval.
func wilsonCI(wins, n int) (p, lo, hi float64) {
	if n == 0 {
		return 0, 0, 0
	}
	p = float64(wins) / float64(n)
	const z = 1.959963984540054
	den := 1 + z*z/float64(n)
	center := (p + z*z/(2*float64(n))) / den
	rad := z / den * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n)))
	return p, math.Max(0, center-rad), math.Min(1, center+rad)
}

// seatLists is game g's two deck lists in engine seat order and deck-file
// order (the order the engine's libraries are built in): the hero sits in
// seat g%2, as draftlab.GauntletGame seats it.
func seatLists(hero, opp draftlab.Deck, g int) [2][]string {
	list := func(d draftlab.Deck) []string {
		var out []string
		for _, e := range d.File.Cards {
			for k := 0; k < e.Count; k++ {
				out = append(out, e.Name)
			}
		}
		return out
	}
	if g%2 == 1 {
		return [2][]string{list(opp), list(hero)}
	}
	return [2][]string{list(hero), list(opp)}
}
