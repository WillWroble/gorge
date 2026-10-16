package main

import (
	"fmt"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/searchprobe"
)

// loadBelief builds a :closedlist seat's belief over the opponent's list: every
// deck of split in poolPath (the TRAIN split by default, so an eval game's
// opponent -- an eval deck -- is never in it), resolved from decksDir. A deck that
// does not resolve is skipped and counted; a belief with no deck is an error.
func loadBelief(reg *cards.Registry, decksDir, poolPath, split string) (*searchprobe.Belief, int, error) {
	pool, err := readPool(poolPath, split)
	if err != nil {
		return nil, 0, err
	}
	dc := &deckCache{reg: reg, dir: decksDir, m: map[string][]*cards.Card{}}
	decks := make([][]*cards.Card, 0, len(pool))
	skipped := 0
	for _, p := range pool {
		d, err := dc.get(p.Name)
		delete(dc.m, p.Name)
		if err != nil {
			skipped++
			continue
		}
		decks = append(decks, d)
	}
	if len(decks) == 0 {
		return nil, skipped, fmt.Errorf("belief: no deck of split %q in %s resolved", split, poolPath)
	}
	b, err := searchprobe.NewBelief(decks)
	return b, skipped, err
}
