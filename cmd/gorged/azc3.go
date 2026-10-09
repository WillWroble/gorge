package main

// Serving az-c3 (docs/superpowers/reports/2026-10-09-az-c3-hosted.md).
//
// The policy is a network trained on one deck distribution (17lands FDN
// limited decks) and measured on nothing else, so the server keeps it there:
// -az-c3-decks names the decks it may play, those decks are reserved for it,
// and a game against it deals from them and from nothing else. The flags
// come as a pair for that reason, and the policy does not exist without them.

import (
	"errors"
	"fmt"

	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/host/httpapi"
)

// enableAZC3 validates the az-c3 flags and registers the policy. It runs
// before the policy flags are resolved: gorged offers every registered entry
// by default, so the entry has to exist, or stay absent, by then.
func (c config) enableAZC3() error {
	switch {
	case c.az3Net == "" && c.az3Decks == "":
		return nil
	case c.az3Net == "":
		return errors.New("-az-c3-decks needs -az-c3-net")
	case c.az3Decks == "":
		return errors.New("-az-c3-net needs -az-c3-decks: the network was trained on one deck distribution and the server keeps the bot to it")
	case !c.vsbot:
		return errors.New("-az-c3-net needs -vsbot: az-c3 is reachable only through the play-vs-bot flow")
	case !c.autoMana || !c.botAutoPayMana:
		return errors.New("-az-c3-net needs -auto-mana and -bot-auto-mana (both default on): az-c3 pays its mana through the engine's automatic payment plans")
	}
	return azc3.Enable(c.az3Net)
}

// azc3Pool loads the decks az-c3 may play from -az-c3-decks: their ids (the
// pool createGame deals from) and their catalogue entries, tagged with the
// policy that owns them. A deck that names a commander is refused (the net
// never saw a Commander game), and so is an id that shadows a deck in -decks,
// because the loader resolves a table's decks by id alone. No flag, no pool.
func (c config) azc3Pool(reg *cards.Registry, cmdPool, conPool []string) ([]string, []httpapi.DeckInfo, error) {
	if c.az3Decks == "" {
		return nil, nil, nil
	}
	commander, constructed, err := splitDecks(reg, c.az3Decks)
	if err != nil {
		return nil, nil, fmt.Errorf("-az-c3-decks: %w", err)
	}
	if len(commander) > 0 {
		return nil, nil, fmt.Errorf("-az-c3-decks: %q names a commander; az-c3 plays 1v1 constructed decks only", commander[0])
	}
	if len(constructed) == 0 {
		return nil, nil, fmt.Errorf("-az-c3-decks: no deck files in %s", c.az3Decks)
	}
	for _, id := range constructed {
		if containsDeck(cmdPool, id) || containsDeck(conPool, id) {
			return nil, nil, fmt.Errorf("-az-c3-decks: deck %q has the same id as a deck in -decks", id)
		}
	}
	catalogue, err := loadDeckCatalogue(c.az3Decks, nil, constructed)
	if err != nil {
		return nil, nil, fmt.Errorf("-az-c3-decks: %w", err)
	}
	for i := range catalogue {
		catalogue[i].Pool = azc3.Policy
	}
	return constructed, catalogue, nil
}

// deckLoaders resolves a table's deck ids from -decks, or from the az-c3
// directory for the ids in its pool; the two directories never share an id.
func deckLoaders(reg *cards.Registry, dir, az3Dir string, az3Pool []string) func(string) (host.Deck, error) {
	main := deckLoader(reg, dir)
	if az3Dir == "" {
		return main
	}
	az := deckLoader(reg, az3Dir)
	return func(name string) (host.Deck, error) {
		if containsDeck(az3Pool, name) {
			return az(name)
		}
		return main(name)
	}
}

// reservedFor returns the bot policy a deck is reserved for, "" when none.
func (c config) reservedFor(id string) string {
	for policy, pool := range c.policyPools {
		if containsDeck(pool, id) {
			return policy
		}
	}
	return ""
}
