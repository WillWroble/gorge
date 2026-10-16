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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/deck"
	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draftbelief"
	"github.com/adams-shaun/gorge/limited/draftfeat"
	"github.com/adams-shaun/gorge/limited/pickbot"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// The belief subcommand measures how well a seat can guess a pod mate's
// deck from what it legally saw (docs/superpowers/reports/
// 2026-10-15-draft-belief.md). Every pod is drafted with an obsDrafter on
// each seat, which records the seat's own PickViews and nothing else; the
// draft-informed belief (limited/draftbelief) reads only that log, the
// public set table and the revealed cards. The true decks are read only to
// score the beliefs.

// beliefSalt tags the reveal-order streams.
const beliefSalt = 0xBE11EF

type beliefOpts struct {
	corpus, set, collation string
	seed                   uint64
	pods                   int
	league                 []leagueSpec
	builder, bbuilder      seatAgent
	k                      int
	l17Dir, l17Pool        string
	l17Max                 int
	sealed                 int
	levels                 []int
	r1                     int
	workers                int
	out                    string
	beliefsOut             string
	samplerKind            string
	beta, kappa, eps       float64
	over                   int
	debug                  bool
	calib                  string
	calibMF                string
	values                 string
	types                  string
	// beliefplay
	games                      int
	azNet                      string
	azSims, azPanics           int
	controlSealed, allowFaults bool
	treat                      string
	pool                       draftbelief.PoolModel
	smc                        draftbelief.SMCOptions
	noRej                      bool
}

func parseBelief(args []string) (beliefOpts, error) {
	var o beliefOpts
	var league, builder, bbuilder, levels string
	fs := flag.NewFlagSet("draftrl belief", flag.ContinueOnError)
	fs.StringVar(&o.corpus, "corpus", ".cards", "card corpus directory")
	fs.StringVar(&o.set, "set", "FDN", "set code")
	fs.StringVar(&o.collation, "collation", string(setdata.DefaultCollation), "booster collation")
	fs.Uint64Var(&o.seed, "seed", 3_000_000, "first pod seed")
	fs.IntVar(&o.pods, "pods", 50, "4-seat pods")
	fs.StringVar(&league, "league", "17l", "drafter specs drawn per seat (rollout's -league)")
	fs.StringVar(&builder, "builder", "17l", "the seats' true builder (best23 ratings): 17l | ovr:PATH#FIELD")
	fs.StringVar(&bbuilder, "belief-builder", "17l", "the builder the draft belief assumes: 17l | ovr:PATH#FIELD")
	fs.IntVar(&o.k, "k", 64, "sampled decks per draft belief")
	fs.StringVar(&o.l17Dir, "l17-dir", "/mnt/sata/gorge-training/dzgorge-repro/inputs/decks", "17Lands deck directory (belief a)")
	fs.StringVar(&o.l17Pool, "l17-pool", "/mnt/sata/gorge-training/dzgorge-repro/inputs/pool.tsv", "pool.tsv naming its train split")
	fs.IntVar(&o.l17Max, "l17-max", 3000, "cap on the 17Lands decks a metric reads per colour mask (evenly strided)")
	fs.IntVar(&o.sealed, "sealed", 500, "random sealed-pool decks (belief b, registerAZ's default)")
	fs.StringVar(&levels, "levels", "0,1,3,5,8,12", "revealed nonland counts at which to score")
	fs.IntVar(&o.r1, "r1", 8, "nonland cards game 1 revealed (Bo3 / cross-round)")
	fs.IntVar(&o.workers, "workers", 8, "concurrent seat pairs")
	fs.StringVar(&o.out, "out", "", "output directory (required)")
	fs.StringVar(&o.beliefsOut, "beliefs-out", "", "optional: write each pair's level-0 draft belief decks here (for the play gate)")
	fs.StringVar(&o.samplerKind, "sampler", "exact", "draft-belief sampler: exact (joint pairs enumerated; 4-seat pods) | gibbs (joint, MCMC) | indep (per-set, pairs from draftfeat marginals)")
	fs.Float64Var(&o.beta, "beta", 0, "pick-model beta override (0 = table default)")
	fs.Float64Var(&o.kappa, "kappa", -1, "pick-model kappa override (<0 = table default)")
	fs.Float64Var(&o.eps, "eps", 0.05, "reveal conditioning: weight per revealed card the unforced build left out")
	fs.StringVar(&o.calib, "calib", "", "calibration grid \"b1,b2;k1,k2\": print the pick model's fit to the true assignments per (beta, kappa) and exit")
	fs.BoolVar(&o.noRej, "no-reject", false, "ablation: drop the first-sighting rejection likelihood")
	fs.StringVar(&o.calibMF, "calib-mf", "", "pool-model calibration grid \"b1,b2;k1,k2\"")
	o.pool = draftbelief.DefaultPool
	fs.Float64Var(&o.pool.Beta, "pool-beta", draftbelief.DefaultPool.Beta, "pool model beta")
	fs.Float64Var(&o.pool.Kappa, "pool-kappa", draftbelief.DefaultPool.Kappa, "pool model kappa")
	fs.IntVar(&o.pool.Iters, "pool-iters", draftbelief.DefaultPool.Iters, "pool model mean-field iterations")
	o.smc = draftbelief.DefaultSMC
	fs.IntVar(&o.smc.Particles, "smc-particles", draftbelief.DefaultSMC.Particles, "smc: particles")
	fs.IntVar(&o.smc.Runs, "smc-runs", draftbelief.DefaultSMC.Runs, "smc: independent filters combined by evidence")
	fs.Float64Var(&o.smc.Beta, "smc-beta", draftbelief.DefaultSMC.Beta, "smc: pool-model beta")
	fs.Float64Var(&o.smc.Kappa, "smc-kappa", draftbelief.DefaultSMC.Kappa, "smc: pool-model kappa")
	fs.StringVar(&o.values, "belief-values", "", "the card values the opponent pick model assumes: empty = 17Lands, or ovr:PATH#FIELD")
	fs.StringVar(&o.types, "belief-types", "", "smc: drafter value types, |-separated (17l | ovr:PATH#FIELD); empty = the table's values only")
	fs.IntVar(&o.games, "games", 2, "beliefplay: games per pod pair (sides alternate); each played twice, beliefs swapped")
	fs.StringVar(&o.treat, "treat", "draft", "beliefplay: treatment belief: draft | oracle (the opponent's real list: an upper bound, never legal)")
	fs.StringVar(&o.azNet, "az-net", "", "beliefplay: the .dzgw network")
	fs.IntVar(&o.azSims, "az-sims", 25, "beliefplay: simulations per searched decision")
	fs.BoolVar(&o.controlSealed, "control-sealed", false, "beliefplay: control belief = draftrl's random sealed-pool decks instead of the 17Lands corpus")
	fs.IntVar(&o.azPanics, "az-panics", 3, "beliefplay: print the first N distinct search-panic stacks")
	fs.BoolVar(&o.allowFaults, "allow-az-faults", false, "beliefplay: do not fail on panics / root-refused")
	fs.BoolVar(&o.debug, "debug", false, "print pod 0 seat 0's removed sets with their true takers")
	fs.IntVar(&o.over, "over", 4, "reveal conditioning: pools drawn per kept deck")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	var err error
	if o.league, err = parseLeague(league); err != nil {
		return o, fmt.Errorf("-league: %w", err)
	}
	for _, x := range []struct {
		s   string
		dst *seatAgent
	}{{builder, &o.builder}, {bbuilder, &o.bbuilder}} {
		b, err := parseSeatAgents(x.s, false)
		if err != nil || len(b) != 1 || (b[0].kind != "17l" && b[0].kind != "ovr") {
			return o, fmt.Errorf("builder %q: want 17l or ovr:PATH#FIELD", x.s)
		}
		*x.dst = b[0]
	}
	for _, t := range strings.Split(levels, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil || n < 0 {
			return o, fmt.Errorf("-levels: %q", t)
		}
		o.levels = append(o.levels, n)
	}
	if o.out == "" {
		return o, fmt.Errorf("-out is required")
	}
	return o, nil
}

// obsDrafter wraps a seat's drafter and records the seat's own legal
// observation log: each PickView it was shown and the card it took. It
// consumes no randomness, so wrapping never moves a pick.
type obsDrafter struct {
	inner pickbot.Drafter
	obs   *draftbelief.Obs
}

func (d *obsDrafter) Pick(v pickbot.PickView, r *rand.Rand) string {
	id := d.inner.Pick(v, r)
	d.obs.Add(v, id)
	return id
}

func (d *obsDrafter) PickErr(v pickbot.PickView, r *rand.Rand) (string, error) {
	if fd, ok := d.inner.(draftlab.FallibleDrafter); ok {
		id, err := fd.PickErr(v, r)
		if err == nil {
			d.obs.Add(v, id)
		}
		return id, err
	}
	return d.Pick(v, r), nil
}

// univ numbers every card name a metric touches.
type univ struct {
	mu    sync.Mutex
	lab   *draftlab.Lab
	ord   map[string]int32
	land  []bool
	ident []uint8 // beliefColours
	cid   []uint8 // colour identity (pairOf)
}

func (u *univ) get(name string) int32 {
	u.mu.Lock()
	defer u.mu.Unlock()
	if o, ok := u.ord[name]; ok {
		return o
	}
	o := int32(len(u.land))
	u.ord[name] = o
	c, ok := u.lab.Card(name)
	land, ident, cid := false, uint8(0), uint8(0)
	if ok && len(c.Faces) > 0 {
		land = c.Faces[0].IsLand()
		cid = c.ColourIdentity() & 31
		ident = cid
		for _, f := range c.Faces {
			if f == nil {
				continue
			}
			if mp := f.ManaProduction(); !mp.Any {
				for i := 0; i < 5; i++ {
					if mp.Colour[i] > 0 {
						ident |= 1 << i
					}
				}
			}
		}
	}
	u.land = append(u.land, land)
	u.ident = append(u.ident, ident)
	u.cid = append(u.cid, cid)
	return o
}

func (u *univ) isLand(o int32) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.land[o]
}

// bdeck is one deck as the metrics read it.
type bdeck struct {
	non   []int32 // nonland cards, a sorted multiset
	ident uint8   // colours of every card (Belief's cover mask)
	pair  int     // pairOf
}

func (u *univ) deck(names []string) bdeck {
	var d bdeck
	var counts [5]int
	for _, n := range names {
		o := u.get(n)
		u.mu.Lock()
		land, ident, cid := u.land[o], u.ident[o], u.cid[o]
		u.mu.Unlock()
		d.ident |= ident
		if land {
			continue
		}
		d.non = append(d.non, o)
		for i := 0; i < 5; i++ {
			if cid&(1<<i) != 0 {
				counts[i]++
			}
		}
	}
	sort.Slice(d.non, func(i, j int) bool { return d.non[i] < d.non[j] })
	d.pair = pairFromCounts(counts)
	return d
}

func pairFromCounts(counts [5]int) int {
	idx := []int{0, 1, 2, 3, 4}
	sort.SliceStable(idx, func(a, b int) bool { return counts[idx[a]] > counts[idx[b]] })
	if counts[idx[0]] == 0 {
		return -1
	}
	a, b := idx[0], idx[1]
	if a > b {
		a, b = b, a
	}
	for p, pr := range draftfeat.Pairs {
		if pr[0] == a && pr[1] == b {
			return p
		}
	}
	return -1
}

// corpusBelief is a fixed list corpus with Belief's colour cover.
type corpusBelief struct {
	decks []bdeck
	cover [32][]int32
	max   int
}

func newCorpusBelief(decks []bdeck, max int) *corpusBelief {
	b := &corpusBelief{decks: decks, max: max}
	for i, d := range decks {
		for m := 0; m < 32; m++ {
			if uint8(m)&^d.ident == 0 {
				b.cover[m] = append(b.cover[m], int32(i))
			}
		}
	}
	return b
}

// cands is what Belief.draw draws uniformly from: the lists covering mask,
// else all of them (strided down to max).
func (b *corpusBelief) cands(mask uint8) []*bdeck {
	idx := b.cover[mask&31]
	var out []*bdeck
	if len(idx) == 0 {
		step := 1
		if b.max > 0 && len(b.decks) > b.max {
			step = (len(b.decks) + b.max - 1) / b.max
		}
		for i := 0; i < len(b.decks); i += step {
			out = append(out, &b.decks[i])
		}
		return out
	}
	step := 1
	if b.max > 0 && len(idx) > b.max {
		step = (len(idx) + b.max - 1) / b.max
	}
	for i := 0; i < len(idx); i += step {
		out = append(out, &b.decks[idx[i]])
	}
	return out
}

// score is one belief's quality against one true deck at one reveal state.
type score struct {
	PairAcc, Prec, Rec, LL, TopK float64
	Cands                        int
}

// metric scores candidate decks (uniform) against the truth: H is the
// true deck's unrevealed nonland multiset, rn the revealed nonland one.
func metric(cands []*bdeck, rn, h []int32, truePair int, nUniv int) score {
	var s score
	s.Cands = len(cands)
	if len(cands) == 0 || len(h) == 0 {
		return s
	}
	counts := map[int32]float64{}
	tot := 0.0
	np, nprec := 0, 0
	for _, d := range cands {
		dp := msDiff(d.non, rn)
		inter := msInter(dp, h)
		if len(dp) > 0 {
			s.Prec += float64(inter) / float64(len(dp))
			nprec++
		}
		s.Rec += float64(inter) / float64(len(h))
		if d.pair == truePair {
			np++
		}
		for _, o := range dp {
			counts[o]++
		}
		tot += float64(len(dp))
	}
	n := float64(len(cands))
	if nprec > 0 {
		s.Prec /= float64(nprec)
	}
	s.Rec /= n
	s.PairAcc = float64(np) / n
	const eps = 0.01
	for _, o := range h {
		p := eps / float64(nUniv)
		if tot > 0 {
			p += (1 - eps) * counts[o] / tot
		}
		s.LL += math.Log(p)
	}
	s.LL /= float64(len(h))
	// Top-k: the k most likely names, k = distinct names of h.
	hd := distinct(h)
	type kv struct {
		o int32
		c float64
	}
	var ranked []kv
	for o, c := range counts {
		ranked = append(ranked, kv{o, c})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].c != ranked[j].c {
			return ranked[i].c > ranked[j].c
		}
		return ranked[i].o < ranked[j].o
	})
	in := map[int32]bool{}
	for _, o := range hd {
		in[o] = true
	}
	hits := 0
	for i := 0; i < len(ranked) && i < len(hd); i++ {
		if in[ranked[i].o] {
			hits++
		}
	}
	s.TopK = float64(hits) / float64(len(hd))
	return s
}

func msDiff(a, b []int32) []int32 {
	out := make([]int32, 0, len(a))
	j := 0
	for _, x := range a {
		for j < len(b) && b[j] < x {
			j++
		}
		if j < len(b) && b[j] == x {
			j++
			continue
		}
		out = append(out, x)
	}
	return out
}

func msInter(a, b []int32) int {
	i, j, n := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			n++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return n
}

func distinct(a []int32) []int32 {
	var out []int32
	for i, x := range a {
		if i == 0 || x != a[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// seatTruth is one pod seat's real deck (scoring only).
type seatTruth struct {
	names   []string // full 40, one per copy
	d       bdeck
	mainIDs map[string]bool
}

// reveal is a prefix of one game's reveal stream: the opponent's 40-card
// list in a seeded order, cut after the n-th nonland card.
type reveal struct {
	names []string
	rn    []int32
	mask  uint8
}

func (u *univ) revealAt(order []string, n int) reveal {
	var r reveal
	if n == 0 {
		return r
	}
	k := 0
	for _, name := range order {
		o := u.get(name)
		u.mu.Lock()
		land, ident := u.land[o], u.ident[o]
		u.mu.Unlock()
		r.mask |= ident
		if !land {
			r.names = append(r.names, name)
			r.rn = append(r.rn, o)
			k++
			if k == n {
				break
			}
		}
	}
	sort.Slice(r.rn, func(i, j int) bool { return r.rn[i] < r.rn[j] })
	return r
}

func shuffled(names []string, r *rand.Rand) []string {
	out := append([]string(nil), names...)
	r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// sampler turns draft-belief pools into decks with the assumed builder.
type sampler struct {
	lab   *draftlab.Lab
	set   string
	b     *pickbot.Best23
	table *draftfeat.SetTable
}

var basicNames = [5]string{"Plains", "Island", "Swamp", "Mountain", "Forest"}

// decks builds each pool, forcing the known names into the main deck (a
// card the seat has seen the opponent play is in its deck, whatever the
// builder would have chosen) by swapping out the lowest-rated nonland.
func (s *sampler) decks(pools [][]limited.CardRef, known []string) [][]string {
	out, _ := s.decksMissing(pools, known)
	return out
}

// decksMissing is decks plus, per deck, how many known cards the unforced
// build left out (the reveal likelihood's exponent).
func (s *sampler) decksMissing(pools [][]limited.CardRef, known []string) ([][]string, []int) {
	out := make([][]string, 0, len(pools))
	miss := make([]int, 0, len(pools))
	for _, pool := range pools {
		d := s.b.Build(s.set, pool)
		byID := map[string]string{}
		for _, c := range pool {
			byID[c.ID] = c.Name
		}
		var main []string
		for _, id := range d.Main {
			main = append(main, byID[id])
		}
		miss = append(miss, missingKnown(main, known))
		main = s.force(main, known)
		for i, n := range d.Basics {
			for k := 0; k < n; k++ {
				main = append(main, basicNames[i])
			}
		}
		out = append(out, main)
	}
	return out, miss
}

func missingKnown(main, known []string) int {
	have := map[string]int{}
	for _, n := range main {
		have[n]++
	}
	m := 0
	for _, n := range known {
		if have[n] > 0 {
			have[n]--
		} else {
			m++
		}
	}
	return m
}

func (s *sampler) val(name string) float64 {
	if i := s.table.Index(name); i >= 0 {
		return s.table.Cards[i].V
	}
	return -10
}

func (s *sampler) force(main, known []string) []string {
	have := map[string]int{}
	for _, n := range main {
		have[n]++
	}
	need := map[string]int{}
	for _, n := range known {
		need[n]++
	}
	keys := make([]string, 0, len(need))
	for n := range need {
		keys = append(keys, n)
	}
	sort.Strings(keys)
	for _, n := range keys {
		for have[n] < need[n] {
			// Replace the lowest-rated card not itself needed.
			worst, wi := math.Inf(1), -1
			for i, m := range main {
				if have[m] <= need[m] {
					continue
				}
				if c, ok := s.lab.Card(m); ok && len(c.Faces) > 0 && c.Faces[0].IsLand() {
					continue
				}
				if v := s.val(m); v < worst {
					worst, wi = v, i
				}
			}
			if wi < 0 {
				main = append(main, n)
			} else {
				have[main[wi]]--
				main[wi] = n
			}
			have[n]++
		}
	}
	return main
}

// collationFreq is each table row's expected copies per pack.
func collationFreq(t *draftfeat.SetTable, set string, coll setdata.Collation) ([]float64, error) {
	spec, err := setdata.LoadCollation(set, coll)
	if err != nil {
		return nil, err
	}
	freq := make([]float64, len(t.Cards))
	cfg := spec.Config
	for _, v := range cfg.Variants {
		pv := float64(v.Weight) / float64(cfg.TotalWeight)
		for name, cnt := range v.Contents {
			sh := cfg.Sheets[name]
			if sh.TotalWeight == 0 {
				continue
			}
			for _, sc := range sh.Cards {
				if i := t.Index(sc.Name); i >= 0 {
					freq[i] += pv * float64(cnt) * float64(sc.Weight) / float64(sh.TotalWeight)
				}
			}
		}
	}
	for i := range t.Cards {
		if t.Cards[i].Types&draftfeat.TypeBasic != 0 {
			freq[i] = 0
		}
	}
	return freq, nil
}

// beliefRow is one (pod, seat, opponent, belief, phase, level) score.
type beliefRow struct {
	Pod, Seat, Opp, Off int
	Belief              string
	Phase               string // g1 | g2 (Bo3 game 2 after game 1's reveals) | xr (cross-round exclusion)
	Level               int
	score
}

// flowRow is one (seat, opponent) card-flow summary.
type flowRow struct {
	Pod, Seat, Opp, Off int
	PairTrue            int
	PairMAP             int // draftfeat's posterior
	LogQTrue            float64
	PairMAPGibbs        int // the joint sampler's latent pair
	LogQTrueGibbs       float64
	ObservedP50         int     // observed cards with P(off took it) >= 0.5
	ObservedP99         int     // ... >= 0.99
	Unseen              int     // the opponent's picks the seat never saw
	MainObserved        float64 // share of the true main's nonland cards the seat saw at all
	MainP50             float64 // share it assigns to this opponent with P >= 0.5
	MeanPTrueMain       float64 // mean P(off took it) over observed true-main cards
}

func belief(args []string, out io.Writer) error {
	o, err := parseBelief(args)
	if err != nil {
		return err
	}
	start := time.Now()
	lab, err := draftlab.Open(o.corpus, draftlab.Options{MaxObjs: 2000, Workers: 1})
	if err != nil {
		return err
	}
	coll := setdata.Collation(o.collation)
	rc := &ratingsCache{set: o.set, m: map[string]*pickbot.RatingsOverride{}, log: func(s string) { fmt.Fprintln(out, s) }}
	bov, err := rc.get(o.builder)
	if err != nil {
		return err
	}
	bbov, err := rc.get(o.bbuilder)
	if err != nil {
		return err
	}
	table, freq, types, err := beliefModel(lab, o, coll, rc, out)
	if err != nil {
		return err
	}
	u := &univ{lab: lab, ord: map[string]int32{}}
	nUniv := 0
	for _, c := range table.Cards {
		if c.Types&draftfeat.TypeLand == 0 {
			nUniv++
		}
	}

	// Belief a: the 17Lands train split.
	reg, err := cards.SharedCorpus(o.corpus)
	if err != nil {
		return err
	}
	var l17 []bdeck
	{
		raw, err := os.ReadFile(o.l17Pool)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(raw), "\n") {
			v := strings.Split(line, "\t")
			if i == 0 || len(v) < 2 || v[1] != "train" {
				continue
			}
			_, cs, err := deck.Load(reg, filepath.Join(o.l17Dir, v[0]+".json"))
			if err != nil {
				continue
			}
			names := make([]string, 0, len(cs))
			for _, c := range cs {
				if c != nil && len(c.Faces) > 0 && c.Faces[0] != nil {
					names = append(names, c.Faces[0].Name)
				}
			}
			l17 = append(l17, u.deck(names))
		}
	}
	beliefA := newCorpusBelief(l17, o.l17Max)
	// Belief b: registerAZ's default.
	var sealedDecks []bdeck
	for i := 0; i < o.sealed; i++ {
		pool, err := lab.SealedPool(o.set, coll, beliefSeedBase+uint64(i), 6)
		if err != nil {
			return err
		}
		d, err := lab.RandomDeck(pool, beliefSeedBase+uint64(i))
		if err != nil {
			return err
		}
		var names []string
		for _, e := range d.File.Cards {
			for k := 0; k < e.Count; k++ {
				names = append(names, e.Name)
			}
		}
		sealedDecks = append(sealedDecks, u.deck(names))
	}
	beliefB := newCorpusBelief(sealedDecks, 0)
	fmt.Fprintf(out, "belief: %d 17Lands decks, %d sealed decks, table %s, loaded in %.1fs\n", len(l17), len(sealedDecks), table.Hash(), time.Since(start).Seconds())

	// Draft every pod with observation recorders.
	type podData struct {
		seed     uint64
		obs      [4]*draftbelief.Obs
		truth    [4]seatTruth
		drafters [4]string
		owner    map[string]int
	}
	pods := make([]podData, o.pods)
	for p := 0; p < o.pods; p++ {
		podSeed := o.seed + uint64(p)
		packs, err := lab.OpenPod(o.set, coll, podSeed, 4, 3)
		if err != nil {
			return err
		}
		seats := seatPod(len(o.league), podSeed, false, nil, -1, 0)
		drafters := make([]draftlab.NamedDrafter, 4)
		pd := &pods[p]
		pd.seed = podSeed
		for s := range drafters {
			d, err := newLeagueDrafter(lab, o.league[seats[s]], o.set, rc)
			if err != nil {
				return err
			}
			pd.obs[s] = &draftbelief.Obs{Seats: 4}
			d.Drafter = &obsDrafter{inner: d.Drafter, obs: pd.obs[s]}
			drafters[s] = d
			pd.drafters[s] = o.league[seats[s]].text
		}
		dr, err := lab.Draft(packs, drafters, podSeed)
		if err != nil {
			return err
		}
		if o.debug {
			for s := range drafters {
				if md, ok := drafters[s].Drafter.(*obsDrafter).inner.(*modelDrafter); ok {
					defer func(s, lat int, pd *podData) {
						fmt.Fprintf(out, "debug latent pod %d seat %d latent %s deck %s\n", pd.seed, s, pairName(lat), pairName(pd.truth[s].d.pair))
					}(s, md.pair, pd)
				}
			}
		}
		pd.owner = map[string]int{}
		for s, pool := range dr.Pools {
			for _, c := range pool {
				pd.owner[c.ID] = s
			}
		}
		if o.debug && p == 0 {
			fl, err := draftbelief.Analyze(*pd.obs[0], table, draftbelief.Options{Freq: freq})
			if err != nil {
				return err
			}
			fl.Debug(out, func(id string) int { return pd.owner[id] })
			defer func() {
				var tp [draftbelief.MaxOpp]int
				for off := 1; off < 4; off++ {
					tp[off-1] = pd.truth[off].d.pair
				}
				lo, mp := fl.JointLogOdds(tp)
				fmt.Fprintf(out, "debug joint: truth %v MAP %v log odds %.2f\n", tp, mp, lo)
				for off := 1; off < 4; off++ {
					q := fl.PairExact(off)
					fmt.Fprintf(out, "debug off %d true pair %s q:", off, pairName(pd.truth[off].d.pair))
					for a := range q {
						if q[a] > 0.01 {
							fmt.Fprintf(out, " %s=%.2f", pairName(a), q[a])
						}
					}
					fmt.Fprintf(out, " | deck %v\n", pd.truth[off].d.non)
				}
			}()
		}
		if o.debug {
			defer func(pools [][]limited.CardRef, pd *podData) {
				for s, pool := range pools {
					pm := uint8(0)
					if pr := pd.truth[s].d.pair; pr >= 0 {
						pm = draftfeat.PairMask[pr]
					}
					var on, tot [3]int
					for i, c := range pool {
						o := u.get(c.Name)
						if u.isLand(o) {
							continue
						}
						cid := u.cid[o]
						if cid == 0 {
							continue
						}
						rd := i / 14
						tot[rd]++
						if cid&^pm == 0 {
							on[rd]++
						}
					}
					fmt.Fprintf(out, "debug onpair pod %d seat %d %s: %d/%d %d/%d %d/%d\n", pd.seed, s, pairName(pd.truth[s].d.pair), on[0], tot[0], on[1], tot[1], on[2], tot[2])
				}
			}(dr.Pools, pd)
		}
		for s, pool := range dr.Pools {
			b, err := lab.Builder(draftlab.AgentRef{Name: "best23"})
			if err != nil {
				return err
			}
			if bov != nil {
				b.Builder.(*pickbot.Best23).Ratings = bov
			}
			dk, err := lab.Build(pool, b)
			if err != nil {
				return err
			}
			var names []string
			for _, e := range dk.File.Cards {
				for c := 0; c < e.Count; c++ {
					names = append(names, e.Name)
				}
			}
			ids := map[string]bool{}
			for _, id := range dk.Build.Main {
				ids[id] = true
			}
			pd.truth[s] = seatTruth{names: names, d: u.deck(names), mainIDs: ids}
		}
	}
	draftDone := time.Now()
	fmt.Fprintf(out, "belief: drafted %d pods in %.1fs\n", o.pods, draftDone.Sub(start).Seconds())
	if o.calib != "" || o.calibMF != "" {
		return calibrate(out, o, table, freq, pods2calib(pods, func(i int) (*draftbelief.Obs, [4]int, map[string]int) {
			var pr [4]int
			for s := range pr {
				pr[s] = pods[i/4].truth[s].d.pair
			}
			return pods[i/4].obs[i%4], pr, pods[i/4].owner
		}, len(pods)*4))
	}

	type job struct{ p, s, t int }
	var jobs []job
	for p := range pods {
		for s := 0; s < 4; s++ {
			for t := 0; t < 4; t++ {
				if s != t {
					jobs = append(jobs, job{p, s, t})
				}
			}
		}
	}
	rows := make([][]beliefRow, len(jobs))
	flows := make([]flowRow, len(jobs))
	errs := make([]error, len(jobs))
	var beliefDump sync.Map
	next := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < o.workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bb, _ := lab.Builder(draftlab.AgentRef{Name: "best23"})
			b23 := bb.Builder.(*pickbot.Best23)
			if bbov != nil {
				b23.Ratings = bbov
			}
			smp := &sampler{lab: lab, set: o.set, b: b23, table: table}
			for i := range next {
				jb := jobs[i]
				rows[i], flows[i], errs[i] = scorePair(o, u, nUniv, table, freq, types, smp, beliefA, beliefB,
					pods[jb.p].seed, jb.p, jb.s, jb.t, pods[jb.p].obs, &pods[jb.p].truth, &beliefDump)
			}
		}()
	}
	for i := range jobs {
		next <- i
	}
	close(next)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("pod %d seat %d opp %d: %w", jobs[i].p, jobs[i].s, jobs[i].t, err)
		}
	}
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return err
	}
	var all []beliefRow
	for _, r := range rows {
		all = append(all, r...)
	}
	if err := writeJSONL(filepath.Join(o.out, "rows.jsonl"), len(all), func(i int) any { return &all[i] }); err != nil {
		return err
	}
	if err := writeJSONL(filepath.Join(o.out, "flow.jsonl"), len(flows), func(i int) any { return &flows[i] }); err != nil {
		return err
	}
	if o.beliefsOut != "" {
		if err := os.MkdirAll(o.beliefsOut, 0o755); err != nil {
			return err
		}
		var keys []string
		beliefDump.Range(func(k, _ any) bool { keys = append(keys, k.(string)); return true })
		sort.Strings(keys)
		for _, k := range keys {
			v, _ := beliefDump.Load(k)
			b, _ := json.Marshal(v)
			if err := os.WriteFile(filepath.Join(o.beliefsOut, k+".json"), b, 0o644); err != nil {
				return err
			}
		}
	}
	summarise(out, all, flows)
	fmt.Fprintf(out, "belief: %d pairs scored in %.1fs; wrote %s\n", len(jobs), time.Since(draftDone).Seconds(), o.out)
	return nil
}

func writeJSONL(path string, n int, at func(i int) any) error {
	return writeFile(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		for i := 0; i < n; i++ {
			if err := enc.Encode(at(i)); err != nil {
				return err
			}
		}
		return nil
	})
}

// scorePair scores every belief for seat s about pod mate t. Only obs[s]
// (and, for the cross-round phase, s's own earlier games' reveals) feeds
// the draft belief; truth is read for scoring and for generating reveals.
func scorePair(o beliefOpts, u *univ, nUniv int, table *draftfeat.SetTable, freq []float64, types [][]float64, smp *sampler,
	a, b *corpusBelief, podSeed uint64, p, s, t int, obs [4]*draftbelief.Obs, truth *[4]seatTruth, dump *sync.Map) ([]beliefRow, flowRow, error) {
	off := (t - s + 4) % 4
	fl, err := draftbelief.Analyze(*obs[s], table, draftbelief.Options{Freq: freq, Types: types})
	if err != nil {
		return nil, flowRow{}, err
	}
	if o.samplerKind == "mf" && fl.Exact() {
		fl.MeanField(o.pool)
	}
	tr := &truth[t]
	r := rand.New(rand.NewPCG(podSeed, beliefSalt<<32|uint64(s)<<8|uint64(t)))
	sample := func(c *draftbelief.Constraints, n int, r *rand.Rand) [][]limited.CardRef {
		if o.samplerKind == "indep" {
			return fl.Sample(off, n, c, r)
		}
		if o.samplerKind == "oracle-pairs" {
			var tp [draftbelief.MaxOpp]int
			for x := 1; x < 4; x++ {
				tp[x-1] = max(truth[(s+x)%4].d.pair, 0)
			}
			return fl.SampleGivenPairs(off, n, tp, r)
		}
		if o.samplerKind == "mf" && fl.Exact() {
			return fl.SampleMF(off, n, c, r)
		}
		if o.samplerKind == "smc" {
			return fl.SampleSMC(off, n, c, o.smc, r)
		}
		if o.samplerKind == "exact" && fl.Exact() {
			return fl.SampleExact(off, n, c, r)
		}
		return fl.SampleGibbs(off, n, c, draftbelief.DefaultGibbs, r)
	}
	// The flow summary.
	fr := flowRow{Pod: p, Seat: s, Opp: t, Off: off, PairTrue: tr.d.pair}
	q := fl.Posterior(off)
	best := 0
	for i := range q {
		if q[i] > q[best] {
			best = i
		}
	}
	fr.PairMAP = best
	if tr.d.pair >= 0 {
		fr.LogQTrue = math.Log(math.Max(q[tr.d.pair], 1e-12))
	}
	_, unseen := fl.Taken(off)
	fr.Unseen = unseen
	pOf := map[string]float64{}
	{
		const n = 256
		for _, pool := range sample(nil, n, rand.New(rand.NewPCG(podSeed, beliefSalt<<32|4<<20|uint64(s)<<8|uint64(t)))) {
			for _, c := range pool {
				if c.ID != "" && c.ID[0] != 'u' && c.ID[0] != 'x' {
					pOf[c.ID] += 1.0 / n
				}
			}
		}
		for _, pv := range pOf {
			if pv >= 0.5 {
				fr.ObservedP50++
			}
			if pv >= 0.99 {
				fr.ObservedP99++
			}
		}
	}
	nMain, nObs, nP50, sumP := 0, 0, 0, 0.0
	for id := range tr.mainIDs {
		nMain++
		if pv, ok := pOf[id]; ok {
			nObs++
			sumP += pv
			if pv >= 0.5 {
				nP50++
			}
		}
	}
	if nMain > 0 {
		fr.MainObserved = float64(nObs) / float64(nMain)
		fr.MainP50 = float64(nP50) / float64(nMain)
	}
	if nObs > 0 {
		fr.MeanPTrueMain = sumP / float64(nObs)
	}

	mk := func(decks [][]string) []bdeck {
		out := make([]bdeck, len(decks))
		for i, d := range decks {
			out[i] = u.deck(d)
		}
		return out
	}
	cover := func(ds []bdeck, mask uint8) []*bdeck {
		var out []*bdeck
		for i := range ds {
			if mask&^ds[i].ident == 0 {
				out = append(out, &ds[i])
			}
		}
		if len(out) == 0 {
			for i := range ds {
				out = append(out, &ds[i])
			}
		}
		return out
	}
	drawDecks := func(c *draftbelief.Constraints, known []string) []bdeck {
		n := o.k
		if len(known) > 0 {
			n = o.k * max(o.over, 1)
		}
		lists, miss := smp.decksMissing(sample(c, n, r), known)
		if len(known) == 0 {
			return mk(lists)
		}
		w := make([]float64, len(lists))
		for i, m := range miss {
			w[i] = math.Pow(o.eps, float64(m))
		}
		keep := make([][]string, 0, o.k)
		for _, i := range systematic(w, o.k, r) {
			keep = append(keep, lists[i])
		}
		return mk(keep)
	}
	var rows []beliefRow
	add := func(name, phase string, lvl int, cands []*bdeck, rn []int32) {
		h := msDiff(tr.d.non, rn)
		rows = append(rows, beliefRow{Pod: p, Seat: s, Opp: t, Off: off, Belief: name, Phase: phase, Level: lvl,
			score: metric(cands, rn, h, tr.d.pair, nUniv)})
	}

	// Game 1 (and Bo1): reveals from one seeded order.
	g1 := shuffled(tr.names, rand.New(rand.NewPCG(podSeed, beliefSalt<<32|1<<20|uint64(s)<<8|uint64(t))))
	g2 := shuffled(tr.names, rand.New(rand.NewPCG(podSeed, beliefSalt<<32|2<<20|uint64(s)<<8|uint64(t))))
	base := drawDecks(nil, nil)
	{
		var cnt [draftfeat.NPairs + 1]int
		for _, d := range base {
			cnt[d.pair+1]++
		}
		bp := 0
		for i := range cnt {
			if cnt[i] > cnt[bp] {
				bp = i
			}
		}
		fr.PairMAPGibbs = bp - 1
		if tr.d.pair >= 0 {
			fr.LogQTrueGibbs = math.Log(math.Max(float64(cnt[tr.d.pair+1])/float64(len(base)), 1.0/float64(2*len(base))))
		}
	}
	if dump != nil && o.beliefsOut != "" {
		var lists [][]string
		pools := sample(nil, o.k, rand.New(rand.NewPCG(podSeed, beliefSalt<<32|3<<20|uint64(s)<<8|uint64(t))))
		lists = smp.decks(pools, nil)
		dump.Store(fmt.Sprintf("p%d-s%d-t%d", podSeed, s, t), lists)
	}
	for _, lvl := range o.levels {
		rv := u.revealAt(g1, lvl)
		add("a", "g1", lvl, a.cands(rv.mask), rv.rn)
		add("b", "g1", lvl, b.cands(rv.mask), rv.rn)
		add("c", "g1", lvl, cover(base, rv.mask), rv.rn)
		cp := base
		if lvl > 0 {
			c := &draftbelief.Constraints{}
			c.Known[off-1] = rv.names
			cp = drawDecks(c, rv.names)
		}
		add("c+", "g1", lvl, cover(cp, rv.mask), rv.rn)
	}

	// Bo3 game 2: game 1 revealed r1 nonland cards.
	r1 := u.revealAt(g1, o.r1)
	for _, lvl := range []int{0, 3} {
		rv := u.revealAt(g2, lvl)
		both := rv.mask | r1.mask
		add("a", "g2", lvl, a.cands(rv.mask), rv.rn)
		add("a+r1col", "g2", lvl, a.cands(both), rv.rn)
		cp := base
		if lvl > 0 {
			c := &draftbelief.Constraints{}
			c.Known[off-1] = rv.names
			cp = drawDecks(c, rv.names)
		}
		add("c+", "g2", lvl, cover(cp, rv.mask), rv.rn)
		known := namesUnionMax(r1.names, rv.names)
		c := &draftbelief.Constraints{}
		c.Known[off-1] = known
		add("d", "g2", lvl, cover(drawDecks(c, known), both), rv.rn)
	}

	// Cross round: the seat has already played the other two pod mates
	// (r1 nonland reveals each, from its own games against them). Their
	// cards are not this opponent's: exclusion only.
	{
		c := &draftbelief.Constraints{}
		for x := 0; x < 4; x++ {
			if x == s || x == t {
				continue
			}
			xo := (x - s + 4) % 4
			gx := shuffled(truth[x].names, rand.New(rand.NewPCG(podSeed, beliefSalt<<32|1<<20|uint64(s)<<8|uint64(x))))
			c.Known[xo-1] = u.revealAt(gx, o.r1).names
		}
		for _, lvl := range []int{0, 3} {
			rv := u.revealAt(g1, lvl)
			cc := *c
			cc.Known[off-1] = rv.names
			add("c+x", "xr", lvl, cover(drawDecks(&cc, rv.names), rv.mask), rv.rn)
		}
	}
	return rows, fr, nil
}

func namesUnionMax(a, b []string) []string {
	ca := map[string]int{}
	for _, n := range a {
		ca[n]++
	}
	cb := map[string]int{}
	for _, n := range b {
		cb[n]++
	}
	out := append([]string(nil), a...)
	for n, c := range cb {
		for k := ca[n]; k < c; k++ {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// summarise prints the metric tables (mean ± standard error over pairs).
func summarise(out io.Writer, rows []beliefRow, flows []flowRow) {
	type key struct {
		phase, belief string
		level         int
	}
	type acc struct {
		n       int
		sum, sq [5]float64
		cands   float64
	}
	m := map[key]*acc{}
	var keys []key
	for _, r := range rows {
		k := key{r.Phase, r.Belief, r.Level}
		a, ok := m[k]
		if !ok {
			a = &acc{}
			m[k] = a
			keys = append(keys, k)
		}
		v := [5]float64{r.PairAcc, r.Prec, r.Rec, r.LL, r.TopK}
		for i, x := range v {
			a.sum[i] += x
			a.sq[i] += x * x
		}
		a.cands += float64(r.Cands)
		a.n++
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].phase != keys[j].phase {
			return keys[i].phase < keys[j].phase
		}
		if keys[i].level != keys[j].level {
			return keys[i].level < keys[j].level
		}
		return keys[i].belief < keys[j].belief
	})
	fmt.Fprintf(out, "%-5s %-8s %3s %6s | %-13s | %-13s | %-13s | %-14s | %-13s | %s\n", "phase", "belief", "N", "pairs", "pair acc", "world prec", "world recall", "LL/card", "top-k", "cands")
	for _, k := range keys {
		a := m[k]
		n := float64(a.n)
		f := func(i int) string {
			mu := a.sum[i] / n
			se := math.Sqrt(math.Max(0, a.sq[i]/n-mu*mu) / n)
			return fmt.Sprintf("%.3f±%.3f", mu, se)
		}
		fmt.Fprintf(out, "%-5s %-8s %3d %6d | %-13s | %-13s | %-13s | %-14s | %-13s | %.0f\n", k.phase, k.belief, k.level, a.n, f(0), f(1), f(2), f(3), f(4), a.cands/n)
	}
	if len(flows) == 0 {
		return
	}
	byOff := map[int][]flowRow{}
	for _, fr := range flows {
		byOff[fr.Off] = append(byOff[fr.Off], fr)
	}
	fmt.Fprintf(out, "flow: off | pairs | pair MAP acc | mean log q(true) | obs P>=.5 | obs P>=.99 | unseen picks | true main seen | main P>=.5 | mean P on seen main\n")
	for off := 1; off < 4; off++ {
		fs := byOff[off]
		var accg, lqg float64
		for _, fr := range fs {
			if fr.PairMAPGibbs == fr.PairTrue {
				accg++
			}
			lqg += fr.LogQTrueGibbs
		}
		var acc, lq, p50, p99, un, ms, mp, mpt float64
		for _, fr := range fs {
			if fr.PairMAP == fr.PairTrue {
				acc++
			}
			lq += fr.LogQTrue
			p50 += float64(fr.ObservedP50)
			p99 += float64(fr.ObservedP99)
			un += float64(fr.Unseen)
			ms += fr.MainObserved
			mp += fr.MainP50
			mpt += fr.MeanPTrueMain
		}
		n := float64(len(fs))
		fmt.Fprintf(out, "flow: %3d | sampled-deck pair MAP acc %.3f | mean log share(true pair) %.3f\n", off, accg/float64(len(fs)), lqg/float64(len(fs)))
		fmt.Fprintf(out, "flow: %3d | %5d | %.3f | %.3f | %.1f | %.1f | %.1f | %.3f | %.3f | %.3f\n", off, len(fs), acc/n, lq/n, p50/n, p99/n, un/n, ms/n, mp/n, mpt/n)
	}
}

// systematic draws k indices proportional to w (systematic resampling).
func systematic(w []float64, k int, r *rand.Rand) []int {
	tot := 0.0
	for _, x := range w {
		tot += x
	}
	out := make([]int, 0, k)
	if tot <= 0 {
		for i := 0; i < k; i++ {
			out = append(out, r.IntN(len(w)))
		}
		return out
	}
	step := tot / float64(k)
	u := r.Float64() * step
	acc, j := 0.0, 0
	for i := 0; i < k; i++ {
		x := u + float64(i)*step
		for j < len(w)-1 && acc+w[j] <= x {
			acc += w[j]
			j++
		}
		out = append(out, j)
	}
	return out
}

type calibItem struct {
	obs   *draftbelief.Obs
	pairs [4]int // true deck pair per absolute seat
	owner map[string]int
	seat  int
}

func pods2calib(_ any, at func(i int) (*draftbelief.Obs, [4]int, map[string]int), n int) []calibItem {
	out := make([]calibItem, n)
	for i := range out {
		ob, pr, ow := at(i)
		out[i] = calibItem{obs: ob, pairs: pr, owner: ow, seat: i % 4}
	}
	return out
}

// calibrate grid-searches the pick model on simulated pods (public model
// fitting; never per-event state): the true-order log-likelihood given
// true pairs, and the exact joint's pair MAP accuracy.
func calibrate(out io.Writer, o beliefOpts, base *draftfeat.SetTable, freq []float64, items []calibItem) error {
	if o.calibMF != "" {
		return calibrateMF(out, o, base, freq, items)
	}
	bs, ks, _ := strings.Cut(o.calib, ";")
	parse := func(x string) []float64 {
		var v []float64
		for _, t := range strings.Split(x, ",") {
			f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
			if err == nil {
				v = append(v, f)
			}
		}
		return v
	}
	for _, beta := range parse(bs) {
		for _, kappa := range parse(ks) {
			m := base.Model
			m.Beta, m.Kappa = beta, kappa
			t, err := draftfeat.NewSetTable(base.Set, base.Cards, m, base.Expected)
			if err != nil {
				return err
			}
			var ll float64
			var nll, hit, nh int
			var lq float64
			for _, it := range items {
				fl, err := draftbelief.Analyze(*it.obs, t, draftbelief.Options{Freq: freq, NoReject: o.noRej})
				if err != nil {
					return err
				}
				var pr [draftbelief.MaxOpp]int
				for off := 1; off < 4; off++ {
					pr[off-1] = max(it.pairs[(it.seat+off)%4], 0)
				}
				owner := func(id string) int {
					if s, ok := it.owner[id]; ok {
						return (s - it.seat + 4) % 4
					}
					return -1
				}
				l, n := fl.TrueOrderLogLik(owner, pr)
				ll += l
				nll += n
				for off := 1; off < 4; off++ {
					q := fl.PairExact(off)
					b := 0
					for i := range q {
						if q[i] > q[b] {
							b = i
						}
					}
					if b == pr[off-1] {
						hit++
					}
					lq += math.Log(math.Max(q[pr[off-1]], 1e-6))
					nh++
				}
			}
			fmt.Fprintf(out, "calib beta %.2f kappa %.2f | true-order loglik/set %.3f | exact pair MAP acc %.3f | mean log q(true pair) %.3f\n",
				beta, kappa, ll/float64(max(nll, 1)), float64(hit)/float64(max(nh, 1)), lq/float64(max(nh, 1)))
		}
	}
	return nil
}

func parseFloats(x string) []float64 {
	var v []float64
	for _, t := range strings.Split(x, ",") {
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err == nil {
			v = append(v, f)
		}
	}
	return v
}

// calibrateMF grid-searches the pool model: the mean field's log posterior
// of the true orders and its colour-pair accuracy.
func calibrateMF(out io.Writer, o beliefOpts, t *draftfeat.SetTable, freq []float64, items []calibItem) error {
	bs, ks, _ := strings.Cut(o.calibMF, ";")
	for _, beta := range parseFloats(bs) {
		for _, kappa := range parseFloats(ks) {
			pm := o.pool
			pm.Beta, pm.Kappa = beta, kappa
			var ll float64
			var nll, hit, nh int
			var own1, ownll float64
			var ownn int
			for _, it := range items {
				a, b, c := draftbelief.OwnPickFit(*it.obs, t, pm)
				own1, ownll, ownn = own1+a, ownll+b, ownn+c
			}
			fmt.Fprintf(out, "calib-own beta %.2f kappa %.2f | own picks top1 %.3f loglik/pick %.3f\n", beta, kappa, own1/float64(ownn), ownll/float64(ownn))
			for _, it := range items {
				if o.debug {
					break
				}
				fl, err := draftbelief.Analyze(*it.obs, t, draftbelief.Options{Freq: freq})
				if err != nil {
					return err
				}
				fl.MeanField(pm)
				owner := func(id string) int {
					if s, ok := it.owner[id]; ok {
						return (s - it.seat + 4) % 4
					}
					return -1
				}
				l, n := fl.TrueOrderMF(owner)
				ll += l
				nll += n
				for off := 1; off < 4; off++ {
					if fl.PairMF(off) == it.pairs[(it.seat+off)%4] {
						hit++
					}
					nh++
				}
			}
			fmt.Fprintf(out, "calib-mf beta %.2f kappa %.2f | true-order log q/set %.3f | pair acc %.3f\n",
				beta, kappa, ll/float64(max(nll, 1)), float64(hit)/float64(max(nh, 1)))
		}
	}
	return nil
}

// beliefModel is the public model the draft belief reads: the set table
// (pick-model values), the collation frequencies and the value types.
func beliefModel(lab *draftlab.Lab, o beliefOpts, coll setdata.Collation, rc *ratingsCache, out io.Writer) (*draftfeat.SetTable, []float64, [][]float64, error) {
	var vov *pickbot.RatingsOverride
	if o.values != "" {
		va, err := parseSeatAgents(o.values, false)
		if err != nil || len(va) != 1 || va[0].kind != "ovr" {
			return nil, nil, nil, fmt.Errorf("-belief-values %q: want ovr:PATH#FIELD", o.values)
		}
		if vov, err = rc.get(va[0]); err != nil {
			return nil, nil, nil, err
		}
	}
	table, err := defaultSetTable(lab, o.set, coll, vov)
	if err != nil {
		return nil, nil, nil, err
	}
	if o.beta > 0 || o.kappa >= 0 {
		m := table.Model
		if o.beta > 0 {
			m.Beta = o.beta
		}
		if o.kappa >= 0 {
			m.Kappa = o.kappa
		}
		if table, err = draftfeat.NewSetTable(table.Set, table.Cards, m, table.Expected); err != nil {
			return nil, nil, nil, err
		}
		fmt.Fprintf(out, "belief: pick model beta %.2f kappa %.2f\n", m.Beta, m.Kappa)
	}
	freq, err := collationFreq(table, o.set, coll)
	if err != nil {
		return nil, nil, nil, err
	}
	var types [][]float64
	if o.types != "" {
		for _, spec := range strings.Split(o.types, "|") {
			var ov *pickbot.RatingsOverride
			if spec != "17l" {
				va, err := parseSeatAgents(spec, false)
				if err != nil || len(va) != 1 || va[0].kind != "ovr" {
					return nil, nil, nil, fmt.Errorf("-belief-types %q: want 17l or ovr:PATH#FIELD", spec)
				}
				if ov, err = rc.get(va[0]); err != nil {
					return nil, nil, nil, err
				}
			}
			tt, err := defaultSetTable(lab, o.set, coll, ov)
			if err != nil {
				return nil, nil, nil, err
			}
			v := make([]float64, len(table.Cards))
			for i := range table.Cards {
				if j := tt.Index(table.Cards[i].Name); j >= 0 {
					v[i] = tt.Cards[j].V
				}
			}
			types = append(types, v)
		}
		fmt.Fprintf(out, "belief: %d drafter value types\n", len(types))
	}
	return table, freq, types, nil
}
