package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/azredeal"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/internal/azmcts"
	"github.com/adams-shaun/gorge/internal/gonet"
	"github.com/adams-shaun/gorge/internal/memcap"
	"github.com/adams-shaun/gorge/internal/policynet"
	"github.com/adams-shaun/gorge/internal/searchprobe"
	"github.com/adams-shaun/gorge/limited/draftbelief"
	"github.com/adams-shaun/gorge/limited/pickbot"
	"github.com/adams-shaun/gorge/limited/setdata"
	"github.com/adams-shaun/gorge/seat"
)

// The beliefplay subcommand is the paired play gate of the draft-belief
// report: pod mates play each other with the closed-list az pilot on both
// sides, one side's opponent belief the draft-informed one (built from that
// seat's own observation log only) and the other side's the 17Lands
// colour-filtered corpus, then the same game seed again with the beliefs
// swapped. The treatment's score is its mean result over both games.

const beliefPlaySalt = 0xBE11F1

type playRec struct {
	Pod, I, J, G int
	Seed         uint64
	Side         [2]int // pod seat of engine player 0 and 1
	TreatSide    int    // the engine side using the draft belief
	Winner       int    // engine side, -1 = draw/stall
	TreatScore   float64
	Turns        int
	Stall        string
}

func beliefPlay(args []string, out io.Writer) error {
	o, err := parseBelief(args)
	if err != nil {
		return err
	}
	if err := memcap.CheckAzCaps(o.workers, o.azSims); err != nil {
		return err
	}
	start := time.Now()
	lab, err := draftlab.Open(o.corpus, draftlab.Options{AllowDecklistBots: true, MaxObjs: 2000, Workers: 1})
	if err != nil {
		return err
	}
	reg, err := cards.SharedCorpus(o.corpus)
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
	var m *policynet.Model
	if o.azNet != "" {
		if m, err = gonet.Model(o.azNet); err != nil {
			return err
		}
	}
	var control *searchprobe.Belief
	if o.controlSealed {
		var lists [][]*cards.Card
		for i := 0; i < o.sealed; i++ {
			pool, err := lab.SealedPool(o.set, coll, beliefSeedBase+uint64(i), 6)
			if err != nil {
				return err
			}
			d, err := lab.RandomDeck(pool, beliefSeedBase+uint64(i))
			if err != nil {
				return err
			}
			lists = append(lists, deckCards(lab, d))
		}
		if control, err = searchprobe.NewBelief(lists); err != nil {
			return err
		}
	} else if control, err = searchprobe.LoadBeliefDir(reg, o.l17Dir, o.l17Pool); err != nil {
		return err
	}
	stats := &azCount{trace: o.azPanics > 0}
	azmcts.Watch = stats.watch
	tracePanics(o.azPanics)
	player := func(b *searchprobe.Belief) draftlab.NamedPlayer {
		cfg := azc3.ClosedConfig(b)
		cfg.Search.Sims = o.azSims
		return draftlab.NamedPlayer{
			Ref: draftlab.AgentRef{Name: azPolicy},
			New: func(seed uint64) (seat.Seat, error) {
				return azredeal.NewWithModel(bots.Options{Seed: seed, AutoPayMana: true, Deps: bots.Deps{Cards: reg}}, cfg, m)
			},
			ReadsDecklists: true,
		}
	}
	controlPlayer := player(control)
	bb, err := lab.Builder(draftlab.AgentRef{Name: "best23"})
	if err != nil {
		return err
	}
	b23 := bb.Builder.(*pickbot.Best23)
	if bbov != nil {
		b23.Ratings = bbov
	}
	smp := &sampler{lab: lab, set: o.set, b: b23, table: table}

	type podPlay struct {
		seed  uint64
		decks [4]draftlab.Deck
		treat [4][4]draftlab.NamedPlayer // [seat][opp]
	}
	pods := make([]podPlay, o.pods)
	for p := range pods {
		podSeed := o.seed + uint64(p)
		packs, err := lab.OpenPod(o.set, coll, podSeed, 4, 3)
		if err != nil {
			return err
		}
		seats := seatPod(len(o.league), podSeed, false, nil, -1, 0)
		drafters := make([]draftlab.NamedDrafter, 4)
		var obs [4]*draftbelief.Obs
		for s := range drafters {
			d, err := newLeagueDrafter(lab, o.league[seats[s]], o.set, rc)
			if err != nil {
				return err
			}
			obs[s] = &draftbelief.Obs{Seats: 4}
			d.Drafter = &obsDrafter{inner: d.Drafter, obs: obs[s]}
			drafters[s] = d
		}
		dr, err := lab.Draft(packs, drafters, podSeed)
		if err != nil {
			return err
		}
		pp := &pods[p]
		pp.seed = podSeed
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
			dk.Name = fmt.Sprintf("p%d-s%d", podSeed, s)
			pp.decks[s] = dk
		}
		// Each seat's draft belief about each pod mate: its own log only.
		for s := 0; s < 4; s++ {
			fl, err := draftbelief.Analyze(*obs[s], table, draftbelief.Options{Freq: freq, Types: types})
			if err != nil {
				return err
			}
			for t := 0; t < 4; t++ {
				if t == s {
					continue
				}
				if o.treat == "oracle" {
					b, err := searchprobe.NewBelief([][]*cards.Card{deckCards(lab, pp.decks[t])})
					if err != nil {
						return err
					}
					pp.treat[s][t] = player(b)
					continue
				}
				off := (t - s + 4) % 4
				r := rand.New(rand.NewPCG(podSeed, beliefPlaySalt<<32|uint64(s)<<8|uint64(t)))
				lists := smp.decks(fl.SampleSMC(off, o.k, nil, o.smc, r), nil)
				var cl [][]*cards.Card
				for _, l := range lists {
					var cs []*cards.Card
					for _, n := range l {
						c, ok := lab.Card(n)
						if !ok {
							return fmt.Errorf("belief card %q not in corpus", n)
						}
						cs = append(cs, c)
					}
					cl = append(cl, cs)
				}
				b, err := searchprobe.NewBelief(cl)
				if err != nil {
					return err
				}
				pp.treat[s][t] = player(b)
			}
		}
	}
	fmt.Fprintf(out, "beliefplay: %d pods drafted and believed in %.1fs\n", o.pods, time.Since(start).Seconds())

	type job struct {
		p, i, j, g, treatSide int
		seed                  uint64
	}
	var jobs []job
	for p := range pods {
		for i := 0; i < 4; i++ {
			for j := i + 1; j < 4; j++ {
				for g := 0; g < o.games; g++ {
					gs := rand.New(rand.NewPCG(pods[p].seed, beliefPlaySalt<<32|1<<24|uint64(i)<<12|uint64(j)<<8|uint64(g))).Uint64()
					for ts := 0; ts < 2; ts++ {
						jobs = append(jobs, job{p: p, i: i, j: j, g: g, treatSide: ts, seed: gs})
					}
				}
			}
		}
	}
	recs := make([]playRec, len(jobs))
	errs := make([]error, len(jobs))
	next := make(chan int)
	var wg sync.WaitGroup
	gstart := time.Now()
	for w := 0; w < o.workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range next {
				jb := jobs[k]
				a, b := jb.i, jb.j
				if jb.g%2 == 1 {
					a, b = b, a
				}
				pp := &pods[jb.p]
				pa, pb := controlPlayer, controlPlayer
				if jb.treatSide == 0 {
					pa = pp.treat[a][b]
				} else {
					pb = pp.treat[b][a]
				}
				res, err := lab.PlayGame(draftlab.Entrant{Deck: pp.decks[a], Player: pa}, draftlab.Entrant{Deck: pp.decks[b], Player: pb}, jb.seed)
				if err != nil {
					errs[k] = err
					continue
				}
				rec := playRec{Pod: jb.p, I: jb.i, J: jb.j, G: jb.g, Seed: jb.seed, Side: [2]int{a, b}, TreatSide: jb.treatSide,
					Winner: res.Winner, Turns: res.Turns, Stall: res.Stall}
				switch {
				case res.Winner < 0:
					rec.TreatScore = 0.5
				case res.Winner == jb.treatSide:
					rec.TreatScore = 1
				}
				recs[k] = rec
			}
		}()
	}
	for k := range jobs {
		waitPause()
		next <- k
	}
	close(next)
	wg.Wait()
	for k, err := range errs {
		if err != nil {
			return fmt.Errorf("job %d: %w", k, err)
		}
	}
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return err
	}
	if err := writeJSONL(filepath.Join(o.out, "games.jsonl"), len(recs), func(i int) any { return &recs[i] }); err != nil {
		return err
	}
	// Paired score: the two games of one seed and pairing.
	var sum, sq float64
	n := 0
	var tot float64
	draws := 0
	for k := 0; k+1 < len(recs); k += 2 {
		s := (recs[k].TreatScore + recs[k+1].TreatScore) / 2
		sum += s
		sq += s * s
		n++
		tot += recs[k].TreatScore + recs[k+1].TreatScore
	}
	for _, r := range recs {
		if r.Winner < 0 {
			draws++
		}
	}
	mean := sum / float64(n)
	se := math.Sqrt(math.Max(0, sq/float64(n)-mean*mean) / float64(n))
	summary := map[string]any{
		"games": len(recs), "pairs": n, "treat_score": mean, "se": se, "ci95": [2]float64{mean - 1.96*se, mean + 1.96*se},
		"draws": draws, "az": stats.summary(), "wall_s": time.Since(start).Seconds(), "games_s": time.Since(gstart).Seconds(),
	}
	b, _ := json.MarshalIndent(summary, "", " ")
	if err := os.WriteFile(filepath.Join(o.out, "summary.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "beliefplay: %d games (%d seed pairs), draft-belief score %.4f ± %.4f (95%% CI [%.4f, %.4f]), draws %d, games %.0fs\n",
		len(recs), n, mean, se, mean-1.96*se, mean+1.96*se, draws, time.Since(gstart).Seconds())
	fmt.Fprintf(out, "  az: %s\n", stats.summary())
	stats.mu.Lock()
	panics, rr := stats.Panics, stats.RootRefused
	stats.mu.Unlock()
	if (panics > 0 || rr > 0) && !o.allowFaults {
		return fmt.Errorf("az faults: panics %d root-refused %d", panics, rr)
	}
	return nil
}

func deckCards(lab *draftlab.Lab, d draftlab.Deck) []*cards.Card {
	var out []*cards.Card
	for _, e := range d.File.Cards {
		c, ok := lab.Card(e.Name)
		if !ok {
			continue
		}
		for k := 0; k < e.Count; k++ {
			out = append(out, c)
		}
	}
	return out
}
