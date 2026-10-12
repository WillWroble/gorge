//go:build manamodel

// Command manamodelstats measures how often gorge's mana-source census
// withholds a cast the seat could really pay, over bot self-play
// (docs/superpowers/reports/2026-10-10-mana-model-gap.md).
//
// At every posed priority decision it reads the engine (pure reads, no
// Submit, no event, no RNG; -verify replays each game hook-free and requires
// the identical chain head):
//
//   - L1 (every window, rules.Engine.MMProbe): each ordinary cast the offer
//     walk would make with unbounded mana is priced from the floating pool
//     plus three source censuses -- W (the shared free-cost census), P (the
//     auto-pay planner's) and F (every mana ability at printed production,
//     activation cost ignored). A cast is L1-positive for W (P) when F pays
//     it and W (P) does not, attributed to the first excluded cost class
//     that alone pays it.
//   - L2 (sampled, rules.Engine.MMRealize): for an L1-positive cast, clones
//     of the engine search real mana activations until the engine's own
//     floating-pool offer walk offers the cast. Real = confirmed.
//   - the option list each seat's policy sees (auto-pay: the plan candidate
//     list seat.Bot.paymentIntent builds; manual: the raw options), minus
//     pass, concede and mana taps.
//
// Built only with -tags manamodel (it reads rules/zz_manamodel.go).
//
//	go run -tags manamodel ./cmd/manamodelstats -family constructed -mode auto -games 2000 -out c-auto.json
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/botpolicy"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/deck"
	"github.com/adams-shaun/gorge/internal/bench"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/seat"
	"github.com/adams-shaun/gorge/state"
)

func main() { os.Exit(mainExit(os.Args[1:], os.Stdout, os.Stderr)) }

type counts map[string]int64

func (c counts) add(k string, n int64) { c[k] += n }

type example struct {
	Cat  string `json:"cat"`
	Text string `json:"text"`
}

type gameResult struct {
	c        counts
	examples []example
}

type config struct {
	auto      bool
	l2Every   int
	l2Depth   int
	l2Nodes   int
	maxObjs   int
	maxL2Cand int
	perDeck   bool
	maxEx     int
}

type gameState struct {
	cfg     *config
	e       *rules.Engine
	res     *gameResult
	deck    [2]string
	seed    uint64
	l1Win   int // L1-positive windows seen this game (L2 sampling clock)
	flags   map[string]bool
	exCount map[string]int
}

func (gs *gameState) add(seatIdx int, k string, n int64) {
	gs.res.c.add(k, n)
	if gs.cfg.perDeck {
		gs.res.c.add("deck:"+gs.deck[seatIdx]+":"+k, n)
	}
}

func (gs *gameState) flag(k string) { gs.flags[k] = true }

func (gs *gameState) ex(cat, format string, a ...any) {
	if gs.exCount[cat] >= gs.cfg.maxEx {
		return
	}
	gs.exCount[cat]++
	gs.res.examples = append(gs.res.examples, example{Cat: cat,
		Text: fmt.Sprintf("[%s:%s seed %d t%d %s] ", gs.deck[0], gs.deck[1], gs.seed, gs.e.G.Turn, gs.e.G.Step) + fmt.Sprintf(format, a...)})
}

func (gs *gameState) name(id state.ObjID) string {
	if o := gs.e.G.Obj(id); o != nil && o.Face() != nil {
		return o.Face().Name
	}
	return fmt.Sprintf("obj%d", id)
}

// visibleList is the option list the seat's policy ranks, minus pass,
// concede and mana taps: for an auto-pay seat the candidate list
// seat.Bot.paymentIntent builds (legacy options without "activate", plus one
// plan-only cast per planned object whose ordinary cast is not offered).
func visibleList(d *decision.Decision, auto bool) (all, spells int) {
	legacyOrdinary := map[state.ObjID]bool{}
	for _, o := range d.Options {
		switch o.Kind {
		case "pass", "concede", "activate":
			continue
		}
		all++
		if o.Kind != "play_land" {
			spells++
		}
		if o.Kind == "cast" && o.Mode == "" && o.AltCostIndex == 0 {
			legacyOrdinary[o.Obj] = true
		}
	}
	if auto {
		for _, a := range d.PaymentActions {
			if len(a.Plans) == 0 || legacyOrdinary[a.Cast.Object] {
				continue
			}
			all++
			spells++
		}
	}
	return all, spells
}

// answerKind names what the seat actually answered: "payment" (a plan),
// "activate" (a manual mana tap: the auto-pay adapter's manual fallback),
// or the chosen option's kind.
func answerKind(d *decision.Decision, in decision.Intent) string {
	if in.Payment != nil {
		return "payment"
	}
	if len(in.Choices) != 1 {
		return "other"
	}
	for _, o := range d.Options {
		if o.Index == in.Choices[0] {
			return o.Kind
		}
	}
	return "other"
}

func bucket(n int) string {
	switch {
	case n == 0:
		return "0"
	case n == 1:
		return "1"
	}
	return "2+"
}

func (gs *gameState) observe(seatIdx int, d *decision.Decision, in decision.Intent) {
	if d.Kind != decision.KPriority {
		return
	}
	e := gs.e
	p := d.Player
	gs.add(seatIdx, "windows", 1)
	ownMain := e.G.Active == p && e.G.Step.IsMain() && len(e.G.Stack) == 0
	all, spells := visibleList(d, gs.cfg.auto)
	gs.add(seatIdx, "list:all:"+bucket(all), 1)
	gs.add(seatIdx, "list:all:sum", int64(all))
	gs.add(seatIdx, "list:spells:"+bucket(spells), 1)
	gs.add(seatIdx, "list:spells:sum", int64(spells))
	if ownMain {
		gs.add(seatIdx, "ownmain:windows", 1)
		gs.add(seatIdx, "ownmain:list:all:"+bucket(all), 1)
		gs.add(seatIdx, "ownmain:list:all:sum", int64(all))
		gs.add(seatIdx, "ownmain:list:spells:"+bucket(spells), 1)
		gs.add(seatIdx, "ownmain:list:spells:sum", int64(spells))
	}
	if !gs.cfg.auto {
		return
	}
	if gs.cfg.maxObjs > 0 && len(e.G.Objs) > gs.cfg.maxObjs {
		gs.add(seatIdx, "skipped_big_board", 1)
		return
	}
	r := e.MMProbe(p)
	if r.ExcludedW {
		gs.add(seatIdx, "excludedW_windows", 1)
	}
	if r.ExcludedP {
		gs.add(seatIdx, "excludedP_windows", 1)
	}
	if r.Unpriced > 0 {
		gs.add(seatIdx, "unpriced_ability_windows", 1)
	}
	planned := map[state.ObjID]bool{}
	for _, a := range d.PaymentActions {
		if len(a.Plans) > 0 {
			planned[a.Cast.Object] = true
		}
	}
	offered := map[state.ObjID]bool{}
	for _, o := range d.Options {
		if o.Kind == "cast" && o.Mode == "" && o.AltCostIndex == 0 {
			offered[o.Obj] = true
		}
	}
	var l1W, l1P, unknown, nonModel bool
	var pos []rules.MMCandidate
	for _, c := range r.Candidates {
		if !c.Known {
			unknown = true
			gs.add(seatIdx, "cand:unknown", 1)
			continue
		}
		if !c.PayF {
			continue
		}
		gs.add(seatIdx, "cand:payF", 1)
		visible := planned[c.Obj] || offered[c.Obj]
		if c.PayP && !visible {
			// The planner's census pays it but no plan was offered: a
			// planner limit outside the mana model (X, hybrid, a mode...).
			nonModel = true
			gs.add(seatIdx, "cand:payP_unplanned", 1)
			gs.ex("payP_unplanned", "%s: the planner census pays it, no plan or cast offered", c.Name)
		}
		if !c.PayW {
			l1W = true
			gs.add(seatIdx, "cand:L1W", 1)
			gs.add(seatIdx, "cand:L1W:class:"+c.ClassW, 1)
		}
		if !c.PayP {
			l1P = true
			gs.add(seatIdx, "cand:L1P", 1)
			gs.add(seatIdx, "cand:L1P:class:"+c.ClassP, 1)
			if visible {
				gs.add(seatIdx, "cand:L1P_visible_anyway", 1)
			}
		}
		if !c.PayW || !c.PayP {
			pos = append(pos, c)
		}
	}
	if unknown {
		gs.add(seatIdx, "windows_unknown", 1)
	}
	if nonModel {
		gs.add(seatIdx, "windows_payP_unplanned", 1)
	}
	if l1W {
		gs.add(seatIdx, "L1W_windows", 1)
		gs.flag("L1W")
		if ownMain {
			gs.add(seatIdx, "ownmain:L1W_windows", 1)
		}
	}
	if l1P {
		gs.add(seatIdx, "L1P_windows", 1)
		gs.flag("L1P")
		if ownMain {
			gs.add(seatIdx, "ownmain:L1P_windows", 1)
		}
	}
	if len(pos) == 0 {
		return
	}
	gs.l1Win++
	if gs.cfg.l2Every <= 0 || (gs.l1Win-1)%gs.cfg.l2Every != 0 {
		return
	}
	gs.add(seatIdx, "L2_sampled_windows", 1)
	var realW, realP bool
	for i, c := range pos {
		if i >= gs.cfg.maxL2Cand {
			gs.add(seatIdx, "L2_cand_capped", 1)
			break
		}
		res := e.MMRealize(p, c.Obj, gs.cfg.l2Depth, gs.cfg.l2Nodes)
		verdict := "false"
		switch {
		case !res.Known:
			verdict = "unknown"
		case res.Real:
			verdict = "real"
		}
		gs.add(seatIdx, "L2_nodes", int64(res.Nodes))
		if res.Guided {
			gs.add(seatIdx, "L2_real_guided", 1)
		}
		if res.Panics > 0 {
			gs.add(seatIdx, "L2_panics", int64(res.Panics))
		}
		if res.StackGrew && !res.Real {
			gs.add(seatIdx, "L2_false_stack_grew", 1)
		}
		if !c.PayW {
			gs.add(seatIdx, "L2W:"+verdict, 1)
			gs.add(seatIdx, "L2W:"+verdict+":class:"+c.ClassW, 1)
			if res.Real {
				realW = true
			}
		}
		if !c.PayP {
			gs.add(seatIdx, "L2P:"+verdict, 1)
			gs.add(seatIdx, "L2P:"+verdict+":class:"+c.ClassP, 1)
			if res.Real {
				realP = true
				if !planned[c.Obj] && !offered[c.Obj] {
					gs.add(seatIdx, "L2P:real_hidden_from_autopay", 1)
				}
				gs.ex("L2P:real:"+c.ClassP, "%s withheld by the planner census, payable (board mana: %s)", c.Name, gs.boardSources(p, r))
			} else if res.Known {
				gs.ex("L2P:false:"+c.ClassP, "%s L1 proxy over-count (board: %s)", c.Name, gs.boardSources(p, r))
			}
		}
		if !c.PayW && c.PayP && res.Real {
			gs.ex("L2W:real:"+c.ClassW, "%s withheld by the shared census only (board: %s)", c.Name, gs.boardSources(p, r))
		}
	}
	if realW {
		gs.add(seatIdx, "L2W_real_windows", 1)
		gs.flag("L2W")
	}
	if realP {
		gs.add(seatIdx, "L2P_real_windows", 1)
		gs.add(seatIdx, "L2P_real_windows:answer:"+answerKind(d, in), 1)
		gs.flag("L2P")
	}
}

// boardSources names the sources carrying an alternative outside W or P.
func (gs *gameState) boardSources(p state.PlayerID, r rules.MMProbeResult) string {
	var out []string
	for _, u := range r.Units {
		for _, a := range u.Alts {
			if !a.InW || !a.InP {
				tag := ""
				if !a.InP {
					tag = "!P"
				} else {
					tag = "!W"
				}
				out = append(out, fmt.Sprintf("%s(%s,%s)", gs.name(u.ID), a.Class, tag))
				break
			}
		}
	}
	return strings.Join(out, " ")
}

func readPool(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for line := 0; sc.Scan(); line++ {
		if line == 0 {
			continue
		}
		v := strings.Split(sc.Text(), "\t")
		if len(v) >= 1 && v[0] != "" {
			out = append(out, v[0])
		}
	}
	return out, sc.Err()
}

// splitmix64 is the deterministic FDN pair sampler.
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func mainExit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manamodelstats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	family := fs.String("family", "constructed", "constructed (60-card repo decks), commander (100-card repo decks) or fdn (FDN pool decks)")
	mode := fs.String("mode", "auto", "auto (both seats auto-pay; runs L1/L2) or manual (both seats manual tap; option lists only)")
	games := fs.Int("games", 100, "total games (rounded up to whole pairs)")
	gpp := fs.Int("games-per-pair", 0, "games per pair (0: games/pairs rounded up; fdn default 2)")
	seed := fs.Uint64("seed", 7700000, "base seed (dev range; keep clear of held-out [1000000,2000000))")
	workers := fs.Int("workers", 4, "parallel games")
	maxTurns := fs.Int("max-turns", 100, "turn cap")
	maxIntents := fs.Int("max-intents", 20000, "intent cap")
	dir := fs.String("dir", ".cards", "corpus")
	pool := fs.String("pool", "/mnt/sata/gorge-training/dzgorge-repro/inputs/pool.tsv", "fdn pool.tsv")
	fdnDecks := fs.String("fdn-decks", "/mnt/sata/gorge-training/dzgorge-repro/inputs/decks", "fdn deck dir")
	out := fs.String("out", "", "write JSON result here")
	l2Every := fs.Int("l2-every", 1, "run L2 on every Nth L1-positive window of a game (0: never)")
	l2Slack := fs.Int("l2-slack", 2, "L2 free search: activations allowed beyond the F witness length")
	l2Nodes := fs.Int("l2-nodes", 400, "L2 max clones per cast")
	maxL2Cand := fs.Int("l2-max-cands", 4, "L2 casts tried per sampled window")
	maxObjs := fs.Int("max-objects", 600, "skip L1/L2 (counted) once the game holds more objects; 0 = no cap")
	maxEx := fs.Int("examples", 2, "examples kept per category per game")
	verify := fs.Bool("verify", false, "replay every game hook-free and require the identical chain head (observation is pure)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reg, err := testutil.OpenCorpusRegistry(*dir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	auto := false
	switch *mode {
	case "auto":
		auto = true
	case "manual":
	default:
		fmt.Fprintln(stderr, "unknown -mode", *mode)
		return 2
	}
	decks := map[string][]*cards.Card{}
	cmdrs := map[string][]int{}
	var pairs []bench.PairDef
	commander := false
	switch *family {
	case "constructed", "commander":
		commander = *family == "commander"
		var names []string
		for _, n := range testutil.RepoDeckNames() {
			f, err := testutil.LoadRepoDeckFile(n)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			if (len(f.CommanderNames()) > 0) != commander {
				continue
			}
			d, err := testutil.LoadRepoDeck(reg, n)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			decks[n] = d
			if commander {
				if verr := f.ValidateCommander(reg); verr != nil {
					fmt.Fprintln(stderr, verr)
					return 1
				}
				cmdrs[n] = f.CommanderIndices()
			}
			names = append(names, n)
		}
		sort.Strings(names)
		pairs = bench.FullPairs(names)
	case "fdn":
		if *gpp == 0 {
			*gpp = 2
		}
		names, err := readPool(*pool)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		np := (*games + *gpp - 1) / *gpp
		for i := 0; len(pairs) < np && i < 50*np; i++ {
			h := splitmix64(*seed ^ uint64(i)*0x51ed27)
			a, b := names[h%uint64(len(names))], names[(h>>32)%uint64(len(names))]
			if a == b {
				continue
			}
			ok := true
			for _, n := range []string{a, b} {
				if _, have := decks[n]; have {
					continue
				}
				_, d, err := deck.Load(reg, filepath.Join(*fdnDecks, n+".json"))
				if err != nil {
					ok = false
					break
				}
				decks[n] = d
			}
			if ok {
				pairs = append(pairs, bench.PairDef{A: a, B: b})
			}
		}
	default:
		fmt.Fprintln(stderr, "unknown -family", *family)
		return 2
	}
	if len(pairs) == 0 {
		fmt.Fprintln(stderr, "no pairs")
		return 1
	}
	if *gpp == 0 {
		*gpp = (*games + len(pairs) - 1) / len(pairs)
	}
	cfgObs := &config{auto: auto, l2Every: *l2Every, l2Depth: *l2Slack, l2Nodes: *l2Nodes, maxObjs: *maxObjs,
		maxL2Cand: *maxL2Cand, perDeck: *family != "fdn", maxEx: *maxEx}
	ctor := func(s uint64) seat.Seat {
		if auto {
			return seat.NewBot(s).EnableAutoPayMana()
		}
		return seat.NewBot(s)
	}
	results := make([][]*gameResult, len(pairs))
	for i := range results {
		results[i] = make([]*gameResult, *gpp)
	}
	var mu sync.Mutex
	gameCfg := func(pd bench.PairDef, s uint64) rules.Config {
		cfg := rules.Config{Seed: s, Names: []string{pd.A, pd.B}, Decks: [][]*cards.Card{decks[pd.A], decks[pd.B]}}
		if commander {
			cfg.Format = rules.FormatCommander
			cfg.StartingLife = 40
			cfg.Commanders = [][]int{cmdrs[pd.A], cmdrs[pd.B]}
		}
		cfg.Tokens = reg.Tokens
		cfg.NameUniverse = reg.Universe()
		return cfg
	}
	play := func(pos int, s uint64, g int, seats [2]seat.Seat) (bench.Outcome, error) {
		pd := pairs[pos]
		cfg := gameCfg(pd, s)
		res := &gameResult{c: counts{}}
		gs := &gameState{cfg: cfgObs, res: res, deck: [2]string{pd.A, pd.B}, seed: s,
			flags: map[string]bool{}, exCount: map[string]int{}}
		hooks := bench.Hooks{
			Setup: func(e *rules.Engine) { gs.e = e },
			Decision: func(seatIdx int, d *decision.Decision, in decision.Intent, brd *botpolicy.Board) error {
				gs.observe(seatIdx, d, in)
				return nil
			},
		}
		o, e, err := bench.PlayGame(cfg, seats[:], *maxTurns, *maxIntents, hooks)
		if err != nil {
			return o, err
		}
		res.c.add("games", 1)
		if o.StallOn != "" {
			res.c.add("stall:"+o.StallOn, 1)
		}
		for _, k := range []string{"L1W", "L1P", "L2W", "L2P"} {
			if gs.flags[k] {
				res.c.add("games_with:"+k, 1)
			}
		}
		if *verify && e != nil {
			var fresh [2]seat.Seat
			for idx := 0; idx < 2; idx++ {
				fresh[idx] = ctor(s ^ uint64(idx+1))
			}
			_, e2, err := bench.PlayGame(cfg, fresh[:], *maxTurns, *maxIntents, bench.Hooks{})
			if err != nil {
				return o, err
			}
			res.c.add("verify:games", 1)
			if e2.L.Head() != e.L.Head() {
				res.c.add("verify:MISMATCH", 1)
			}
		}
		mu.Lock()
		results[pos][g] = res
		mu.Unlock()
		return o, nil
	}
	if _, err := bench.RunPairs(*seed, *gpp, pairs, ctor, ctor, play, *workers, nil); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	total := counts{}
	var exs []example
	exPerCat := map[string]int{}
	for _, row := range results {
		for _, res := range row {
			if res == nil {
				continue
			}
			for k, v := range res.c {
				total[k] += v
			}
			for _, x := range res.examples {
				if exPerCat[x.Cat] < 12 {
					exPerCat[x.Cat]++
					exs = append(exs, x)
				}
			}
		}
	}
	sort.Slice(exs, func(i, j int) bool {
		if exs[i].Cat != exs[j].Cat {
			return exs[i].Cat < exs[j].Cat
		}
		return exs[i].Text < exs[j].Text
	})
	doc := map[string]any{
		"family": *family, "mode": *mode, "pairs": len(pairs), "games_per_pair": *gpp, "seed": *seed,
		"max_turns": *maxTurns, "l2_every": *l2Every, "counts": total, "examples": exs,
	}
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", " ")
		if err := enc.Encode(doc); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		f.Close()
	}
	keys := make([]string, 0, len(total))
	for k := range total {
		if !strings.HasPrefix(k, "deck:") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(stdout, "%-50s %d\n", k, total[k])
	}
	return 0
}
