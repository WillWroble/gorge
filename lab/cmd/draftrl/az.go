package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/testkit/bench"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/azmcts"
	"github.com/adams-shaun/gorge/bots/azredeal"
	"github.com/adams-shaun/gorge/bots/gonet"
	"github.com/adams-shaun/gorge/bots/policynet"
	"github.com/adams-shaun/gorge/bots/searchprobe"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/lab/draft/draftlab"
	"github.com/adams-shaun/gorge/lab/draft/limited/setdata"
)

// azPolicy is the registry key of the draftrl-only az gauntlet pilot.
const azPolicy = "draftrl-az"

// beliefSeedBase is where the default belief's sealed pools are opened:
// the training seed range (spec §3.0: training seeds start at 1,000,000),
// so no belief deck shares a pack with a gate seed.
const beliefSeedBase = 1_000_000

// azCount tallies azmcts.Watch over the run (lab/cmd/cardval's azCount), so
// fallbacks and search panics are reported instead of hidden.
type azCount struct {
	mu                                                                  sync.Mutex
	trace                                                               bool
	Decisions, Searched, Skipped, FeedStopped, Refused, AllFailed, Sims int
	Completed, Panics, NoWorld                                          int
	// RootRefused is FeedStopped's share whose feed was live but whose
	// honest root refused (azmcts.Stats.RootRefused); FeedStopped keeps
	// counting both, so a true feed stop is FeedStopped - RootRefused.
	RootRefused int
	// Livelocks are simulations whose world tripped the engine's livelock
	// contract, scored as stall draws (azmcts.Stats.Livelocks).
	Livelocks int
}

func (c *azCount) watch(d azmcts.Diag) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d.Stats.FeedStopped > 0 {
		c.FeedStopped++
		c.RootRefused += d.Stats.RootRefused
		return
	}
	c.Decisions++
	if d.Searched {
		c.Searched++
	}
	if d.Refused != "" {
		c.Refused++
	}
	c.Skipped += d.Stats.Skipped
	c.AllFailed += d.Stats.AllFailed
	c.Sims += d.Stats.Simulations
	c.Completed += d.Stats.Completed
	c.Panics += d.Stats.Panics
	c.NoWorld += d.Stats.NoWorld
	c.Livelocks += d.Stats.Livelocks
	if d.Stats.Panics > 0 && c.trace {
		fmt.Fprintf(os.Stderr, "draftrl: az panic x%d: seat seed %d (game seed %d or %d), decision seq %d, turn %d, kind %s\n",
			d.Stats.Panics, d.Seed, d.Seed^1, d.Seed^2, d.Seq, d.Turn, d.Kind)
	}
}

func (c *azCount) summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("decisions %d searched %d refused %d feed-stopped %d (root-refused %d) all-failed %d sims %d completed %d panics %d no-world %d livelocks %d",
		c.Decisions, c.Searched, c.Refused, c.FeedStopped, c.RootRefused, c.AllFailed, c.Sims, c.Completed, c.Panics, c.NoWorld, c.Livelocks)
}

// registerAZ registers the closed-list az pilot (az-c4's search config,
// azc3.ClosedConfig) at o.azSims with the network at o.azNet ("" = no
// network). The belief is LoadBeliefDir(o.azBeliefDir, o.azBeliefPool)
// when a directory is given, else o.azBelief random decks from sealed
// pools opened at training seeds: never a gate deck, never a seat's real
// list.
func registerAZ(lab *draftlab.Lab, o gateOpts) (*azCount, string, error) {
	var m *policynet.Model
	if o.azNet != "" {
		var err error
		if m, err = gonet.Model(o.azNet); err != nil {
			return nil, "", err
		}
	}
	var b *searchprobe.Belief
	var err error
	beliefLabel := ""
	if o.azBeliefDir != "" {
		reg, rerr := cards.SharedCorpus(o.corpus)
		if rerr != nil {
			return nil, "", rerr
		}
		if b, err = searchprobe.LoadBeliefDir(reg, o.azBeliefDir, o.azBeliefPool); err != nil {
			return nil, "", fmt.Errorf("-az-belief-dir: %w", err)
		}
		beliefLabel = "dir " + o.azBeliefDir
	} else {
		lists := make([][]*cards.Card, 0, o.azBelief)
		for i := 0; i < o.azBelief; i++ {
			pool, err := lab.SealedPool(o.set, setdata.Collation(o.collation), beliefSeedBase+uint64(i), 6)
			if err != nil {
				return nil, "", fmt.Errorf("belief pool %d: %w", i, err)
			}
			d, err := lab.RandomDeck(pool, beliefSeedBase+uint64(i))
			if err != nil {
				return nil, "", fmt.Errorf("belief deck %d: %w", i, err)
			}
			var list []*cards.Card
			for _, e := range d.File.Cards {
				c, ok := lab.Card(e.Name)
				if !ok {
					return nil, "", fmt.Errorf("belief: %q not in corpus", e.Name)
				}
				for k := 0; k < e.Count; k++ {
					list = append(list, c)
				}
			}
			lists = append(lists, list)
		}
		if b, err = searchprobe.NewBelief(lists); err != nil {
			return nil, "", err
		}
		beliefLabel = fmt.Sprintf("%d random sealed-pool decks", o.azBelief)
	}
	cfg := azc3.ClosedConfig(b)
	cfg.Search.Sims = o.azSims
	net := "nonet"
	if m != nil {
		net = o.azNet
	}
	stats := &azCount{trace: o.azPanics > 0}
	azmcts.Watch = stats.watch
	tracePanics(o.azPanics)
	bench.FallbackTrace = func(seed uint64, seatIdx int, d *decision.Decision, reason string) {
		fmt.Fprintf(os.Stderr, "draftrl: az fallback: game seed %d, seat %d, decision seq %d kind %s: %s\n", seed, seatIdx, d.Seq, d.Kind, reason)
	}
	if !registered {
		bots.Register(bots.Entry{
			Info: bots.Info{
				Name:        azPolicy,
				Label:       "draftrl az gauntlet pilot",
				Tier:        bots.Experimental,
				Env:         true,
				Search:      true,
				ClosedLists: true,
			},
			New: func(o bots.Options) (seat.Seat, error) {
				return azredeal.NewWithModel(o, current.cfg, current.m)
			},
		})
		registered = true
	}
	current.cfg, current.m = cfg, m
	return stats, fmt.Sprintf("%s sims=%d net=%s belief=%s", azPolicy, o.azSims, net, beliefLabel), nil
}

// registered/current let one process (the tests) run more than one az
// gate: bots.Register takes a name once, so the entry reads the latest
// configuration. A run sets it before any game starts.
var (
	registered bool
	current    struct {
		cfg azmcts.SeatConfig
		m   *policynet.Model
	}
)

// panicLog groups recovered search panics by where they were recovered and
// their message (digits blanked), printing the first limit distinct stacks
// (lab/cmd/cardval's -az-panics).
var panicLog struct {
	mu    sync.Mutex
	limit int
	n     map[string]int
	order []string
}

func tracePanics(limit int) {
	if limit <= 0 {
		azmcts.PanicTrace = nil
		return
	}
	panicLog.limit, panicLog.n, panicLog.order = limit, map[string]int{}, nil
	azmcts.PanicTrace = func(where string, p any, stack []byte) {
		key := where + ": " + strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return '#'
			}
			return r
		}, fmt.Sprint(p))
		panicLog.mu.Lock()
		defer panicLog.mu.Unlock()
		if panicLog.n[key] == 0 {
			panicLog.order = append(panicLog.order, key)
			if len(panicLog.order) <= panicLog.limit {
				fmt.Fprintf(os.Stderr, "draftrl: az panic %s\n  raw: %v\n%s\n", key, p, stack)
			}
		}
		panicLog.n[key]++
	}
}
