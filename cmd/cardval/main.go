// Command cardval is the cold-start card-value spike: it estimates how much
// each card of a set is worth from simulated win/loss alone, with no card
// ratings. It generates random two-colour archetype decks from the set's
// card list, plays them with one pilot policy on both sides, and writes one
// JSON line per game. The analysis (regression, paired differences, the
// comparison with 17Lands) is offline.
//
// Modes:
//
//	random    every game is a fresh random deck against a fresh random deck
//	variance  -decks decks, each playing -per games against fresh random
//	          opponents (measures the between-deck spread of win rate)
//	swap      paired trials: deck A holding card X at slot k, and the same
//	          deck holding card Y at slot k, each play the same opponent with
//	          the same seed, so the two games differ only once X or Y matters
//	replay    re-play one games.jsonl row (-line) and write its two deck
//	          files: the repro path for a faulted game
//	cards     write cards.json only
//
// -hands (on by default) adds each seat's in-hand cards to every row (ha,
// hb): its opening hand after mulligans plus every card it later drew, the
// 17Lands GIH set. -az pilots both seats with the az search instead of
// -pilot (az.go).
//
// -notes adds a census of every engine Note by its source card
// (notes.json), the way to find cards the engine plays approximately.
// Data and analysis scripts live in /mnt/sata/gorge-training/cardval.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/deck"
	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/memcap"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/state"
)

const wubrg = "WUBRG"

// card is one eligible nonland card of the set.
type card struct {
	Name     string `json:"name"`
	Colours  uint8  `json:"colours"` // colour identity, W=1 U=2 B=4 R=8 G=16
	Creature bool   `json:"creature"`
	Pips     [5]int `json:"pips"`
	Types    string `json:"types"`
	MV       int32  `json:"mv"`
	Oracle   string `json:"oracle"`
}

// pair is one two-colour archetype and its eligible card indices.
type pair struct {
	Name              string
	Mask              uint8
	Creatures, Others []int
}

// gameLine is one game's output row. Deck lists are indices into cards.json.
type gameLine struct {
	Game    int    `json:"g"`
	Trial   int    `json:"t,omitempty"`
	Variant int    `json:"v,omitempty"` // swap: 0 = card X, 1 = card Y
	Deck    int    `json:"d,omitempty"` // variance: hero deck id
	Seed    uint64 `json:"seed"`
	PairA   string `json:"pa"`
	PairB   string `json:"pb"`
	A       []int  `json:"a"`
	B       []int  `json:"b"`
	X       int    `json:"x,omitempty"`
	Y       int    `json:"y,omitempty"`
	Slot    int    `json:"k,omitempty"`
	Winner  int    `json:"w"`
	Starter int    `json:"s"`
	Turns   int    `json:"turns"`
	Stall   string `json:"stall,omitempty"`
	Mull    [2]int `json:"mull"`
	HA      []int  `json:"ha,omitempty"` // seat 0's in-hand cards (handLog)
	HB      []int  `json:"hb,omitempty"`
	Err     string `json:"err,omitempty"`
}

func main() {
	var (
		corpus   = flag.String("corpus", ".cards", "card corpus directory")
		set      = flag.String("set", "FDN", "set code")
		mode     = flag.String("mode", "random", "random | variance | swap")
		pilot    = flag.String("pilot", "bot", "player policy for both seats")
		n        = flag.Int("n", 1000, "random: games; swap: trials")
		decks    = flag.Int("decks", 200, "variance: hero decks")
		per      = flag.Int("per", 200, "variance: games per hero deck")
		seed     = flag.Uint64("seed", 1, "master seed")
		workers  = flag.Int("workers", 4, "concurrent games")
		out      = flag.String("out", "", "output directory (games.jsonl, cards.json)")
		creature = flag.Int("creatures", 15, "creatures among the 23 nonland cards")
		notes    = flag.Bool("notes", false, "census every engine Note by its source card into notes.json")
		line     = flag.String("line", "", "replay: a games.jsonl row to replay; writes the two deck files and the result")
		dumplog  = flag.Bool("dumplog", false, "replay: also dump the replayed game's event log to replay-events.json")
		inHand   = flag.Bool("hands", true, "record each seat's in-hand cards (opening hand + draws) per game")
		az       = flag.Bool("az", false, "pilot both seats with the az search (az.go) instead of -pilot")
		azNet    = flag.String("az-net", "", "with -az: the .dzgw network; empty = no network (knows no card)")
		azSims   = flag.Int("az-sims", 25, "with -az: simulations per searched decision")
		azBelief = flag.Int("az-belief", 2000, "with -az: random decks in the closed-list belief")
		azPanics = flag.Int("az-panics", 0, "with -az: print the stack of the first N distinct recovered search panics to stderr, and a census of all of them at the end")
		maxObjs  = flag.Int("max-objs", 2000, "board-size cap: a game past it is a stall, not a 20 GB process (0 = off)")
	)
	flag.Parse()
	if *out == "" {
		fail("-out is required")
	}
	if *az {
		// Fail fast when the memory this run is given cannot hold the az
		// working set: a kernel memcg OOM kill is exactly what the reward
		// loop's stability veto counts, and 2026-10-14's seven-kill retry
		// burst came from a run launched far under its need. Checked before
		// the corpus loads, so the refusal costs nothing. See internal/memcap.
		if err := memcap.CheckAzCaps(*workers, *azSims); err != nil {
			fail("%v", err)
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail("%v", err)
	}
	var nc *noteCensus
	var hl *handLog
	if *notes {
		nc = &noteCensus{by: map[string]map[string]int{}, attributed: map[string]map[string]int{},
			playMiss: map[string][]playMissSet{}, seeds: map[string][]uint64{},
			abortAttr: map[string]map[string]int{}}
	}
	// the az pilot is an Env seat with a closed list: a bot-only game
	opts := draftlab.Options{AllowDecklistBots: *az, MaxObjs: *maxObjs}
	opts.OnEngine = func(seed uint64, eng *rules.Engine) {
		if nc != nil {
			nc.add(seed, eng)
		}
		if hl != nil {
			hl.add(seed, eng)
		}
	}
	if *dumplog {
		if nc != nil {
			fail("-dumplog and -notes are separate modes")
		}
		opts.OnEngine = func(_ uint64, eng *rules.Engine) {
			writeJSON(*out+"/replay-events.json", eng.L.Events)
			writeJSON(*out+"/replay-tallies.json", map[string]any{
				"abort": eng.AbortTally, "legacy_ask": eng.LegacyAskTally})
		}
	}
	lab, err := draftlab.Open(*corpus, opts)
	if err != nil {
		fail("%v", err)
	}
	cs, err := eligible(lab, *set)
	if err != nil {
		fail("%v", err)
	}
	pairs := pairsOf(cs)
	if *inHand {
		hl = newHandLog(cs)
	}
	g := gen{cs: cs, pairs: pairs, creatures: *creature, lab: lab, hands: hl}
	label := *pilot
	if *az {
		if *azPanics > 0 {
			tracePanics(*azPanics)
		}
		if label, err = registerAZ(&g, *azNet, *azSims, *azBelief, *seed); err != nil {
			fail("%v", err)
		}
		*pilot = azPolicy
	}
	ref, err := draftlab.ParseAgent(*pilot)
	if err != nil {
		fail("%v", err)
	}
	player, err := lab.Player(ref)
	if err != nil {
		fail("%v", err)
	}
	g.player = player
	if !*az {
		label = player.Ref.String()
	}
	writeJSON(*out+"/cards.json", cs)
	writeJSON(*out+"/run.json", map[string]any{
		"set": *set, "mode": *mode, "pilot": label, "n": *n,
		"decks": *decks, "per": *per, "seed": *seed, "creatures": *creature,
	})

	if *mode == "cards" {
		return
	}
	if *mode == "replay" {
		replayLine(lab, player, cs, pairs, *line, *out)
		return
	}
	var jobs []func() []gameLine
	switch *mode {
	case "random":
		for i := 0; i < *n; i++ {
			i := i
			jobs = append(jobs, func() []gameLine { return g.randomGame(*seed, i) })
		}
	case "variance":
		for d := 0; d < *decks; d++ {
			for j := 0; j < *per; j++ {
				d, j := d, j
				jobs = append(jobs, func() []gameLine { return g.varianceGame(*seed, d, j, *per) })
			}
		}
	case "swap":
		for i := 0; i < *n; i++ {
			i := i
			jobs = append(jobs, func() []gameLine { return g.swapTrial(*seed, i) })
		}
	default:
		fail("unknown -mode %q", *mode)
	}
	run(jobs, *workers, *out+"/games.jsonl")
	if nc != nil {
		writeJSON(*out+"/notes.json", nc.by)
		writeJSON(*out+"/notes-attributed.json", nc.attributed)
		writeJSON(*out+"/notes-aborts.json", nc.abortAttr)
		writeJSON(*out+"/notes-playmiss.json", nc.playMiss)
		writeJSON(*out+"/notes-seeds.json", nc.seeds)
	}
	if *az {
		writeJSON(*out+"/az.json", &azStats)
		fmt.Fprintf(os.Stderr, "cardval: az: %d decisions, %d searched, %d feed-stopped, %d refused, %.1f sims/searched\n",
			azStats.Decisions, azStats.Searched, azStats.FeedStopped, azStats.Refused, float64(azStats.Sims)/max(1, float64(azStats.Searched)))
		fmt.Fprintf(os.Stderr, "cardval: az: sims %d completed %d panics %d no-world %d all-failed %d\n",
			azStats.Sims, azStats.Completed, azStats.Panics, azStats.NoWorld, azStats.AllFailed)
		panicCensus()
	}
}

// noteCensus counts every engine Note by the name of its source object's
// card and its text (digits blanked so object ids group together). CV-6
// adds three attributions the raw log cannot give:
//   - attributed: rules.Engine's AbortTally / LegacyAskTally, which name the
//     card or ability behind the notes whose event carries no source object
//     (the aborted cast/activation Notes and the default-answered ask),
//   - playMiss: for every "Play found no card to play" firing, the cards
//     exiled from libraries since the previous firing, so a miss whose
//     exiles were all lands reads benign and a missed castable spell shows
//     its name.
type noteCensus struct {
	mu         sync.Mutex
	by         map[string]map[string]int
	attributed map[string]map[string]int
	playMiss   map[string][]playMissSet
	// seeds records the game seeds of the diagnosable notes (the aborted
	// cast/activation Notes, the default-answered asks and the Play misses),
	// bounded so one pathological text cannot balloon the file.
	seeds map[string][]uint64
	// abortAttr attributes every "aborted" Note in the surviving log to the
	// proposal it reversed (the log scan described above).
	abortAttr map[string]map[string]int
}

// playMissSet is one "Play found no card to play" firing and the cards that
// had been exiled from libraries since the previous firing.
type playMissSet struct {
	Exiles []string `json:"exiles"`
}

// blankNoteText is the census's text key: digits blanked so object ids
// group together, truncated to 160 bytes.
func blankNoteText(s string) string {
	t := []byte(s)
	for i, c := range t {
		if c >= '0' && c <= '9' {
			t[i] = '#'
		}
	}
	if len(t) > 160 {
		t = t[:160]
	}
	return string(t)
}

func (n *noteCensus) add(seed uint64, eng *rules.Engine) {
	n.mu.Lock()
	defer n.mu.Unlock()
	var libExiles []string
	// lastProposal is the most recent spell/ability proposal event: the
	// nearest PutOnStack (a spell cast's own push, Obj = the card) or
	// AbilityPush (an activated ability's mint, Obj = the source permanent)
	// before an abort Note names the proposal the abort reversed. A log scan
	// rather than the engine's scratch: the tape kernel's checkpoint Restore
	// rewinds the log over an aborted resolution and re-runs it, so scratch
	// tallies can disagree with the surviving log while the log is always
	// the truth.
	proposal := ""
	for _, e := range eng.L.Events {
		if e.Kind == events.DeclareAttackers {
			// An attacks trigger's dig belongs to its own declaration: a
			// miss's window must not span into earlier triggers whose play
			// ask was posed (and so fired no Note).
			libExiles = nil
			continue
		}
		if e.Kind == events.MoveZone && e.From == state.ZLibrary && e.To == state.ZExile {
			if o := eng.G.Obj(e.Obj); o != nil && o.Face() != nil {
				libExiles = append(libExiles, o.Face().Name)
			} else {
				libExiles = append(libExiles, fmt.Sprintf("obj#%d", e.Obj))
			}
			continue
		}
		if e.Kind == events.PutOnStack || e.Kind == events.AbilityPush {
			proposal = "obj#" + fmt.Sprint(e.Obj)
			if o := eng.G.Obj(e.Obj); o != nil && o.Face() != nil {
				proposal = o.Face().Name
			}
			if e.Kind == events.AbilityPush {
				proposal += " (activation)"
			}
			continue
		}
		if e.Kind != events.Note {
			continue
		}
		text := blankNoteText(e.Text)
		name := "<none>"
		if o := eng.G.Obj(e.Obj); o != nil && o.Card != nil && len(o.Card.Faces) > 0 {
			name = o.Card.Faces[0].Name
		}
		if strings.Contains(e.Text, "Play found no card to play") {
			m := n.playMiss[name]
			if m == nil {
				m = []playMissSet{}
				n.playMiss[name] = m
			}
			n.playMiss[name] = append(m, playMissSet{Exiles: libExiles})
			libExiles = nil
		}
		if strings.Contains(e.Text, "aborted") {
			m := n.abortAttr[text]
			if m == nil {
				m = map[string]int{}
				n.abortAttr[text] = m
			}
			m[proposal]++
		}
		if strings.Contains(e.Text, "aborted") ||
			strings.Contains(e.Text, "ask answered with its default") ||
			strings.Contains(e.Text, "Play found no card to play") {
			key := text + "|" + name
			if strings.Contains(e.Text, "aborted") {
				key = text + "|" + proposal
			}
			if len(n.seeds[key]) < 40 {
				n.seeds[key] = append(n.seeds[key], seed)
			}
		}
		m := n.by[name]
		if m == nil {
			m = map[string]int{}
			n.by[name] = m
		}
		m[text]++
	}
	mergeTally := func(t map[string]int) {
		for key, count := range t {
			text, label, found := strings.Cut(key, "\x1f")
			if !found {
				label = "<unknown>"
			}
			m := n.attributed[label]
			if m == nil {
				m = map[string]int{}
				n.attributed[label] = m
			}
			m[blankNoteText(text)] += count
		}
	}
	mergeTally(eng.AbortTally)
	mergeTally(eng.LegacyAskTally)
}

// eligible is the set's nonland, non-basic cards the corpus carries, in name
// order, with colour identity at most two colours.
func eligible(lab *draftlab.Lab, set string) ([]card, error) {
	names, err := lab.CardNames(set, "")
	if err != nil {
		return nil, err
	}
	var out []card
	for _, nm := range names {
		c, ok := lab.Card(nm)
		if !ok || len(c.Faces) == 0 {
			fmt.Fprintf(os.Stderr, "cardval: skip %q: not in corpus\n", nm)
			continue
		}
		f := c.Faces[0]
		if f.IsLand() {
			continue
		}
		mask := c.ColourIdentity()
		if bitsOf(mask) > 2 {
			continue
		}
		cd := card{Name: nm, Colours: mask, Creature: f.IsCreature(), Types: strings.Join(f.Types, " "), MV: f.ManaValue(), Oracle: f.Oracle}
		for _, face := range c.Faces {
			for i := 0; i < 5; i++ {
				cd.Pips[i] += int(cards.ManaCostChroma(face.ManaCost, 1<<i))
			}
		}
		out = append(out, cd)
	}
	return out, nil
}

// pairsOf builds the ten colour-pair archetypes: a card is eligible for a
// pair when its colour identity is a subset of the pair (colourless cards
// are eligible everywhere).
func pairsOf(cs []card) []pair {
	var ps []pair
	for a := 0; a < 5; a++ {
		for b := a + 1; b < 5; b++ {
			m := uint8(1<<a | 1<<b)
			p := pair{Name: string([]byte{wubrg[a], wubrg[b]}), Mask: m}
			for i, c := range cs {
				if c.Colours&^m != 0 {
					continue
				}
				if c.Creature {
					p.Creatures = append(p.Creatures, i)
				} else {
					p.Others = append(p.Others, i)
				}
			}
			ps = append(ps, p)
		}
	}
	return ps
}

type gen struct {
	cs        []card
	pairs     []pair
	creatures int
	lab       *draftlab.Lab
	player    draftlab.NamedPlayer
	hands     *handLog
}

// randomDeck draws a pair and 23 distinct nonland cards (g.creatures of them
// creatures where the pair has enough), in a random slot order.
func (g *gen) randomDeck(rng *rand.Rand) (pair, []int) {
	p := g.pairs[rng.IntN(len(g.pairs))]
	nc := g.creatures
	if nc > len(p.Creatures) {
		nc = len(p.Creatures)
	}
	no := 23 - nc
	if no > len(p.Others) {
		no = len(p.Others)
		nc = 23 - no
	}
	main := append(sample(rng, p.Creatures, nc), sample(rng, p.Others, no)...)
	rng.Shuffle(len(main), func(i, j int) { main[i], main[j] = main[j], main[i] })
	return p, main
}

// deckFile is the engine list: the nonland cards one entry each in slot
// order, then 17 basics split by the pair's pip counts.
func (g *gen) deckFile(p pair, main []int, name string) draftlab.Deck {
	f := deck.File{Name: name, Format: "limited"}
	var pips [5]int
	for _, i := range main {
		f.Cards = append(f.Cards, deck.Entry{Name: g.cs[i].Name, Count: 1})
		for c := 0; c < 5; c++ {
			pips[c] += g.cs[i].Pips[c]
		}
	}
	var cols []int
	for c := 0; c < 5; c++ {
		if p.Mask&(1<<c) != 0 {
			cols = append(cols, c)
		}
	}
	a, b := pips[cols[0]], pips[cols[1]]
	na := 9
	if a+b > 0 {
		na = (17*a + (a+b)/2) / (a + b)
	}
	if na < 5 {
		na = 5
	}
	if na > 12 {
		na = 12
	}
	f.Cards = append(f.Cards,
		deck.Entry{Name: draftlab.BasicNames[cols[0]], Count: na},
		deck.Entry{Name: draftlab.BasicNames[cols[1]], Count: 17 - na})
	d, err := g.lab.ListDeck(f)
	if err != nil {
		fail("%v", err)
	}
	return d
}

func (g *gen) play(a, b draftlab.Deck, s uint64, line gameLine) gameLine {
	r, err := g.lab.PlayGame(draftlab.Entrant{Deck: a, Player: g.player}, draftlab.Entrant{Deck: b, Player: g.player}, s)
	line.Seed = s
	if err != nil {
		line.Err = err.Error()
		line.Winner = -1
		return line
	}
	line.Winner, line.Starter, line.Turns, line.Stall, line.Mull = r.Winner, r.Starter, r.Turns, r.Stall, r.Mulligans
	if g.hands != nil {
		if h, ok := g.hands.take(s); ok {
			line.HA, line.HB = h[0], h[1]
		}
	}
	return line
}

func (g *gen) randomGame(seed uint64, i int) []gameLine {
	rng := rand.New(rand.NewPCG(seed, 0xCA4D<<32|uint64(i)))
	pa, ma := g.randomDeck(rng)
	pb, mb := g.randomDeck(rng)
	line := gameLine{Game: i, PairA: pa.Name, PairB: pb.Name, A: ma, B: mb}
	return []gameLine{g.play(g.deckFile(pa, ma, "a"), g.deckFile(pb, mb, "b"), rng.Uint64(), line)}
}

func (g *gen) varianceGame(seed uint64, d, j, per int) []gameLine {
	drng := rand.New(rand.NewPCG(seed, 0x7A41<<32|uint64(d)))
	pa, ma := g.randomDeck(drng)
	rng := rand.New(rand.NewPCG(seed^uint64(d+1)*0x9E3779B97F4A7C15, 0x7A42<<32|uint64(j)))
	pb, mb := g.randomDeck(rng)
	line := gameLine{Game: d*per + j, Deck: d, PairA: pa.Name, PairB: pb.Name, A: ma, B: mb}
	a, b := g.deckFile(pa, ma, "a"), g.deckFile(pb, mb, "b")
	// alternate seats so seat order never confounds a deck
	if j%2 == 1 {
		line.PairA, line.PairB, line.A, line.B = pb.Name, pa.Name, mb, ma
		l := g.play(b, a, rng.Uint64(), line)
		l.Deck = -d - 1 // negative: the hero deck is seat 1
		return []gameLine{l}
	}
	return []gameLine{g.play(a, b, rng.Uint64(), line)}
}

// swapTrial plays the paired games of one trial: base deck with X at slot k,
// the same deck with Y at slot k, the same opponent, the same seed.
func (g *gen) swapTrial(seed uint64, i int) []gameLine {
	rng := rand.New(rand.NewPCG(seed, 0x5A4B<<32|uint64(i)))
	pa, ma := g.randomDeck(rng)
	pb, mb := g.randomDeck(rng)
	k := rng.IntN(len(ma))
	x := ma[k]
	in := map[int]bool{}
	for _, c := range ma {
		in[c] = true
	}
	var cand []int
	for _, c := range append(append([]int(nil), pa.Creatures...), pa.Others...) {
		if !in[c] {
			cand = append(cand, c)
		}
	}
	if len(cand) == 0 {
		return nil
	}
	sort.Ints(cand)
	y := cand[rng.IntN(len(cand))]
	my := append([]int(nil), ma...)
	my[k] = y
	s := rng.Uint64()
	db := g.deckFile(pb, mb, "b")
	base := gameLine{Trial: i, PairA: pa.Name, PairB: pb.Name, B: mb, X: x, Y: y, Slot: k}
	lx := base
	lx.Game, lx.A = 2*i, ma
	ly := base
	ly.Game, ly.Variant, ly.A = 2*i+1, 1, my
	return []gameLine{
		g.play(g.deckFile(pa, ma, "a"), db, s, lx),
		g.play(g.deckFile(pa, my, "a"), db, s, ly),
	}
}

// run executes the jobs on workers goroutines and appends every row to path.
func run(jobs []func() []gameLine, workers int, path string) {
	f, err := os.Create(path)
	if err != nil {
		fail("%v", err)
	}
	w := bufio.NewWriter(f)
	var mu sync.Mutex
	var done, games, errs atomic.Int64
	next := atomic.Int64{}
	var wg sync.WaitGroup
	for wk := 0; wk < workers; wk++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(jobs) {
					return
				}
				rows := jobs[i]()
				mu.Lock()
				for _, r := range rows {
					b, _ := json.Marshal(r)
					w.Write(b)
					w.WriteByte('\n')
					games.Add(1)
					if r.Err != "" {
						errs.Add(1)
					}
				}
				if d := done.Add(1); d%10000 == 0 {
					fmt.Fprintf(os.Stderr, "cardval: %d/%d jobs, %d games, %d errors\n", d, len(jobs), games.Load(), errs.Load())
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if err := w.Flush(); err != nil {
		fail("%v", err)
	}
	f.Close()
	fmt.Fprintf(os.Stderr, "cardval: done: %d games, %d errors\n", games.Load(), errs.Load())
}

func sample(rng *rand.Rand, from []int, k int) []int {
	c := append([]int(nil), from...)
	rng.Shuffle(len(c), func(i, j int) { c[i], c[j] = c[j], c[i] })
	return c[:k]
}

func bitsOf(m uint8) int {
	n := 0
	for ; m != 0; m &= m - 1 {
		n++
	}
	return n
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		fail("%v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		fail("%v", err)
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "cardval: "+format+"\n", a...)
	os.Exit(1)
}

// replayLine re-plays one recorded game from its row: the same decks in the
// same slot order, the same seed. It writes a.json and b.json (engine deck
// files) and prints the result or the error.
func replayLine(lab *draftlab.Lab, player draftlab.NamedPlayer, cs []card, pairs []pair, row, out string) {
	var gl gameLine
	if err := json.Unmarshal([]byte(row), &gl); err != nil {
		fail("replay: %v", err)
	}
	g := gen{cs: cs, pairs: pairs, lab: lab, player: player}
	byName := map[string]pair{}
	for _, p := range pairs {
		byName[p.Name] = p
	}
	a := g.deckFile(byName[gl.PairA], gl.A, "a")
	b := g.deckFile(byName[gl.PairB], gl.B, "b")
	writeJSON(out+"/a.json", a.File)
	writeJSON(out+"/b.json", b.File)
	r, err := lab.PlayGame(draftlab.Entrant{Deck: a, Player: player}, draftlab.Entrant{Deck: b, Player: player}, gl.Seed)
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	fmt.Printf("winner %d turns %d stall %q head %s\n", r.Winner, r.Turns, r.Stall, r.Head)
}
