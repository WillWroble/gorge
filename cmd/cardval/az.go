package main

import (
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/azredeal"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/azmcts"
	"github.com/adams-shaun/gorge/internal/gonet"
	"github.com/adams-shaun/gorge/internal/policynet"
	"github.com/adams-shaun/gorge/internal/searchprobe"
	"github.com/adams-shaun/gorge/seat"
)

// azPolicy is the registry key of the cardval-only az search pilot.
const azPolicy = "cardval-az"

// azCount tallies azmcts.Watch over the run, so a pilot whose search never
// ran (every decision routed around it) shows up in az.json instead of
// passing for the scripted bot.
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

var azStats azCount

// registerAZ registers the az search pilot: az-c4's search config
// (azc3.Config: morekinds, autopay, oppnodes, oppfull, topk=6) at sims
// simulations per searched decision, with the network at netPath ("" = no
// network: the heuristic leaf and a uniform prior, which knows no card).
//
// The opponent's list is closed under a belief of beliefDecks random
// archetype decks from this run's own generator, never the opponent's real
// list and never a human deck corpus: on a new set this is all a cold start
// has. The seat is still an Env seat (its search runs a real engine), so the
// lab admits it only with AllowDecklistBots, which this bot-only
// measurement sets.
func registerAZ(g *gen, netPath string, sims, beliefDecks int, seed uint64) (string, error) {
	var m *policynet.Model
	if netPath != "" {
		var err error
		if m, err = gonet.Model(netPath); err != nil {
			return "", err
		}
	}
	rng := rand.New(rand.NewPCG(seed, 0xBE11EF))
	lists := make([][]*cards.Card, 0, beliefDecks)
	for i := 0; i < beliefDecks; i++ {
		p, main := g.randomDeck(rng)
		d := g.deckFile(p, main, "belief")
		var list []*cards.Card
		for _, e := range d.File.Cards {
			c, ok := g.lab.Card(e.Name)
			if !ok {
				return "", fmt.Errorf("belief: %q not in corpus", e.Name)
			}
			for k := 0; k < e.Count; k++ {
				list = append(list, c)
			}
		}
		lists = append(lists, list)
	}
	b, err := searchprobe.NewBelief(lists)
	if err != nil {
		return "", err
	}
	cfg := azc3.ClosedConfig(b)
	cfg.Search.Sims = sims
	net := "nonet"
	if m != nil {
		net = netPath
	}
	azmcts.Watch = azStats.watch
	bots.Register(bots.Entry{
		Info: bots.Info{
			Name:        azPolicy,
			Label:       fmt.Sprintf("cardval az search, sims=%d, net=%s", sims, net),
			Tier:        bots.Experimental,
			Env:         true,
			Search:      true,
			ClosedLists: true,
		},
		New: func(o bots.Options) (seat.Seat, error) { return azredeal.NewWithModel(o, cfg, m) },
	})
	return fmt.Sprintf("%s@sims=%d,net=%s", azPolicy, sims, net), nil
}
