package main

// Serving az-c3 (docs/design/2026-10-09-az-c3-hosted.md).
//
// The policy is a network trained on one deck distribution (17lands FDN
// limited decks) and measured on nothing else, so the server keeps it there:
// -az-c3-decks names the decks it may play, those decks are reserved for it,
// and a game against it deals from them and from nothing else. The flags
// come as a pair for that reason, and the policy does not exist without them.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/server/host"
	"github.com/adams-shaun/gorge/server/host/httpapi"
)

// enableAZC3 validates the az-c3 flags and registers the policy. It runs
// before the policy flags are resolved: gorged offers every registered
// non-decklist entry by default (botpolicies.go), so the entry has to
// exist, or stay absent, by then. az-c3 reads opponents' decklists
// (bots.Info.Env), so it is never offered; it runs only on the bot-only
// demo table (azc3Table), and -az-c3-net requires the operator's
// -allow-decklist-bots opt-in.
func (c config) enableAZC3() error {
	switch {
	case c.az3Net == "" && (c.az3Belief != "" || c.az3BeliefPool != ""):
		return errors.New("-az-c3-belief needs -az-c3-net")
	case c.az3BeliefPool != "" && c.az3Belief == "":
		return errors.New("-az-c3-belief-pool needs -az-c3-belief")
	case c.az3Net == "" && c.az3Decks == "":
		return nil
	case c.az3Net == "":
		return errors.New("-az-c3-decks needs -az-c3-net")
	case c.az3Decks == "":
		return errors.New("-az-c3-net needs -az-c3-decks: the network was trained on one deck distribution and the server keeps the bot to it")
	case !c.allowDecklistBots:
		return errors.New("-az-c3-net needs -allow-decklist-bots: az-c3 reads opponents' decklists and runs only on its bot-only demo table, never against a human")
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
		// The picker shows "name - archetype": without one these decks look like
		// ids among the hand-named ones, and they sort after them.
		if catalogue[i].Archetype == "" {
			catalogue[i].Archetype = "FDN limited, plays vs az-c3"
		}
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

// reservedFor returns the bot policy a deck is reserved for, "" when none;
// a deck shared by az-c3 and a closed-list policy names both (reservedOwners, azclosed.go).
func (c config) reservedFor(id string) string {
	return strings.Join(c.reservedOwners(id), ", ")
}

// azc3Table is the demo's az-c3 bot-versus-bot table: two seats, both az-c3,
// dealt from the az-c3 pool, playing match after match for anyone watching.
// Its id follows the numbered tables. Nil when az-c3 is off or the demo table
// is switched off (-az-c3-demo-table=false).
func (c config) azc3Table(pool []string, vis view.Visibility) *host.TableConfig {
	if c.az3Net == "" || !c.az3DemoTable || len(pool) == 0 {
		return nil
	}
	return &host.TableConfig{
		ID: host.TableID(fmt.Sprintf("t%d", c.tables+1)), Name: "az-c3 vs az-c3 (FDN)", Seats: 2, Decks: pool,
		Seed: c.seed + uint64(c.tables), Pace: c.pace, Spectator: vis, Perpetual: c.perpetual,
		Mulligans: c.mulligans, Format: host.FormatConstructed, BotPolicy: azc3.Policy,
		BotAutoPayMana: c.botAutoPayMana, AutoMana: c.autoMana,
		// The table exists only when az-c3 is enabled, and enabling az-c3
		// required -allow-decklist-bots (enableAZC3): this is the opt-in's
		// one effect. All seats are bots; the host refuses az-c3 at any
		// table with a human even with the bit set.
		AllowDecklistBots: true,
	}
}

// azc3Belief loads -az-c3-belief (with -az-c3-belief-pool's train split, or
// every deck file in the directory) and closes az-c3's opponent lists with
// it. No flag: the open lists, unchanged.
func (c config) azc3Belief(reg *cards.Registry) error {
	if c.az3Belief == "" {
		return nil
	}
	b, err := loadBelief(reg, c.az3Belief, c.az3BeliefPool, "-az-c3-belief", "-az-c3-belief-pool")
	if err != nil {
		return err
	}
	azc3.SetBelief(b)
	return nil
}
