package main

import (
	"fmt"
	"sync"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/azredeal"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/internal/azmcts"
	"github.com/adams-shaun/gorge/internal/gonet"
	"github.com/adams-shaun/gorge/internal/policynet"
	"github.com/adams-shaun/gorge/internal/searchprobe"
	"github.com/adams-shaun/gorge/limited/setdata"
	"github.com/adams-shaun/gorge/seat"
)

// azPolicy is the registry key of the draftrl-only az gauntlet pilot.
const azPolicy = "draftrl-az"

// beliefSeedBase is where the default belief's sealed pools are opened:
// the training seed range (spec §3.0: training seeds start at 1,000,000),
// so no belief deck shares a pack with a gate seed.
const beliefSeedBase = 1_000_000

// azCount tallies azmcts.Watch over the run (cmd/cardval's azCount), so
// fallbacks and search panics are reported instead of hidden.
type azCount struct {
	mu                                                                  sync.Mutex
	Decisions, Searched, Skipped, FeedStopped, Refused, AllFailed, Sims int
	Completed, Panics, NoWorld                                          int
}

func (c *azCount) watch(d azmcts.Diag) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d.Stats.FeedStopped > 0 {
		c.FeedStopped++
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
}

func (c *azCount) summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("decisions %d searched %d refused %d feed-stopped %d all-failed %d sims %d completed %d panics %d no-world %d",
		c.Decisions, c.Searched, c.Refused, c.FeedStopped, c.AllFailed, c.Sims, c.Completed, c.Panics, c.NoWorld)
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
	stats := &azCount{}
	azmcts.Watch = stats.watch
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
