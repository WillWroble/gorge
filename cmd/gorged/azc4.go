package main

// Serving az-c4 (docs/superpowers/reports/2026-10-14-az-c4-hosted.md).
//
// az-c4 is az-c3's search with the C4 network, playing CLOSED opponent lists
// only: it guesses the opponent's deck from a belief corpus and never reads
// the real one, so -- unlike az-c3 -- it is offered for games against a
// human and needs no -allow-decklist-bots. Its flags mirror az-c3's:
// -az-c4-net and -az-c4-decks come as a pair (a network trained on one deck
// distribution plays only those decks), and -az-c4-belief is required, not
// optional. The decks may be the same directory as -az-c3-decks: a shared
// deck is reserved for both policies and listed once.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc4"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/deck"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/host/httpapi"
	"github.com/adams-shaun/gorge/internal/searchprobe"
	"github.com/adams-shaun/gorge/view"
)

// checkAZC4Flags validates the az-c4 flags alone. It runs before anything
// is loaded, so a misconfigured server fails fast.
func (c config) checkAZC4Flags() error {
	switch {
	case c.az4BeliefPool != "" && c.az4Belief == "":
		return errors.New("-az-c4-belief-pool needs -az-c4-belief")
	case c.az4Net == "" && c.az4Decks == "" && c.az4Belief == "":
		return nil
	case c.az4Net == "":
		return errors.New("-az-c4-decks and -az-c4-belief need -az-c4-net")
	case c.az4Decks == "":
		return errors.New("-az-c4-net needs -az-c4-decks: the network was trained on one deck distribution and the server keeps the bot to it")
	case c.az4Belief == "":
		return errors.New("-az-c4-net needs -az-c4-belief: az-c4 plays humans only because it guesses the opponent's list from a belief corpus instead of reading it")
	case !c.autoMana || !c.botAutoPayMana:
		return errors.New("-az-c4-net needs -auto-mana and -bot-auto-mana (both default on): az-c4 pays its mana through the engine's automatic payment plans")
	}
	return nil
}

// enableAZC4 loads -az-c4-belief and registers az-c4 over -az-c4-net with
// its opponents' lists closed under it. It runs before the policy flags are
// resolved (the offered set is read from the registry) and after the corpus
// is open (the belief's decks are resolved against it). No -az-c4-net: no
// policy.
func (c config) enableAZC4(reg *cards.Registry) error {
	if err := c.checkAZC4Flags(); err != nil {
		return err
	}
	if c.az4Net == "" {
		return nil
	}
	b, err := loadBelief(reg, c.az4Belief, c.az4BeliefPool, "-az-c4-belief", "-az-c4-belief-pool")
	if err != nil {
		return err
	}
	return azc4.Enable(c.az4Net, b)
}

// loadBelief builds a closed-list belief from dir: the decks pool (a
// pool.tsv of "deck<TAB>split<TAB>...") names as its train split, or every
// deck file in dir without one. A deck the corpus cannot resolve is skipped.
// dirFlag and poolFlag name the flags in its errors. It is the one reader
// behind -az-c3-belief (azc3Belief) and -az-c4-belief.
func loadBelief(reg *cards.Registry, dir, pool, dirFlag, poolFlag string) (*searchprobe.Belief, error) {
	var names []string
	if pool != "" {
		raw, err := os.ReadFile(pool)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", poolFlag, err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			v := strings.Split(line, "\t")
			if i == 0 || len(v) < 2 || v[1] != "train" {
				continue
			}
			names = append(names, v[0]+".json")
		}
	} else {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dirFlag, err)
		}
		for _, e := range ents {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				names = append(names, e.Name())
			}
		}
	}
	sort.Strings(names)
	var decks [][]*cards.Card
	for _, n := range names {
		_, d, err := deck.Load(reg, filepath.Join(dir, n))
		if err != nil {
			continue // a deck the corpus cannot resolve is no evidence either way
		}
		decks = append(decks, d)
	}
	b, err := searchprobe.NewBelief(decks)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", dirFlag, dir, err)
	}
	return b, nil
}

// azc4Pool loads the decks az-c4 may play from -az-c4-decks, exactly as
// azc3Pool loads az-c3's. A deck id it shares with az-c3's pool must come
// from the same file (the two flags name one directory, or the id would be
// ambiguous to the loader); such a deck is reserved for both policies and is
// not catalogued twice: az3Catalogue's entry is re-tagged in place instead.
// It returns az-c4's ids, the catalogue entries to append (the decks az-c3
// does not already list) and az3Catalogue, possibly re-tagged.
func (c config) azc4Pool(reg *cards.Registry, cmdPool, conPool, az3Pool []string, az3Catalogue []httpapi.DeckInfo) ([]string, []httpapi.DeckInfo, []httpapi.DeckInfo, error) {
	if c.az4Decks == "" {
		return nil, nil, az3Catalogue, nil
	}
	commander, constructed, err := splitDecks(reg, c.az4Decks)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("-az-c4-decks: %w", err)
	}
	if len(commander) > 0 {
		return nil, nil, nil, fmt.Errorf("-az-c4-decks: %q names a commander; az-c4 plays 1v1 constructed decks only", commander[0])
	}
	if len(constructed) == 0 {
		return nil, nil, nil, fmt.Errorf("-az-c4-decks: no deck files in %s", c.az4Decks)
	}
	sameDir := c.az3Decks != "" && filepath.Clean(c.az3Decks) == filepath.Clean(c.az4Decks)
	var fresh []string
	for _, id := range constructed {
		if containsDeck(cmdPool, id) || containsDeck(conPool, id) {
			return nil, nil, nil, fmt.Errorf("-az-c4-decks: deck %q has the same id as a deck in -decks", id)
		}
		if containsDeck(az3Pool, id) {
			if !sameDir {
				return nil, nil, nil, fmt.Errorf("-az-c4-decks: deck %q has the same id as a deck in -az-c3-decks (a different directory); share one directory or rename it", id)
			}
			continue
		}
		fresh = append(fresh, id)
	}
	catalogue, err := loadDeckCatalogue(c.az4Decks, nil, fresh)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("-az-c4-decks: %w", err)
	}
	for i := range catalogue {
		catalogue[i].Pool = azc4.Policy
		if catalogue[i].Archetype == "" {
			catalogue[i].Archetype = "FDN limited, plays vs az-c4"
		}
	}
	retagged := az3Catalogue
	if len(fresh) < len(constructed) {
		retagged = append([]httpapi.DeckInfo(nil), az3Catalogue...)
		for i := range retagged {
			if containsDeck(constructed, retagged[i].ID) {
				retagged[i].Pool = httpapi.JoinPools(retagged[i].Pool, azc4.Policy)
				if retagged[i].Archetype == "FDN limited, plays vs az-c3" {
					retagged[i].Archetype = "FDN limited, plays vs az-c3 or az-c4"
				}
			}
		}
	}
	return constructed, catalogue, retagged, nil
}

// reservedLoader wraps load so the ids in pool resolve from dir. A shared
// directory (az-c3 and az-c4 on one -decks path) is the same loader twice,
// which resolves every shared id identically.
func reservedLoader(reg *cards.Registry, load func(string) (host.Deck, error), dir string, pool []string) func(string) (host.Deck, error) {
	if dir == "" || len(pool) == 0 {
		return load
	}
	az := deckLoader(reg, dir)
	return func(name string) (host.Deck, error) {
		if containsDeck(pool, name) {
			return az(name)
		}
		return load(name)
	}
}

// reservedOwners returns every bot policy a deck is reserved for, sorted;
// reservedFor (azc3.go) joins them.
func (c config) reservedOwners(id string) []string {
	var owners []string
	for policy, pool := range c.policyPools {
		if containsDeck(pool, id) {
			owners = append(owners, policy)
		}
	}
	sort.Strings(owners)
	return owners
}

// reservedPick is the policy a reserved deck picks for a request that names
// none: the first owner that may play a human (one that does not read
// decklists), else the first owner -- which createGame then refuses with
// the decklist rule, as it always has for a deck reserved only for az-c3.
func (c config) reservedPick(id string) string {
	owners := c.reservedOwners(id)
	for _, o := range owners {
		if !bots.ReadsDecklists(o) {
			return o
		}
	}
	if len(owners) > 0 {
		return owners[0]
	}
	return ""
}

// azc4Table is the demo's az-c4 bot-versus-bot table, dealt from the az-c4
// pool, playing match after match for anyone watching; id names it. Nil when
// az-c4 is off or the demo table is switched off (-az-c4-demo-table=false).
// It needs no -allow-decklist-bots: az-c4 never reads a decklist.
func (c config) azc4Table(id host.TableID, seed uint64, pool []string, vis view.Visibility) *host.TableConfig {
	if c.az4Net == "" || !c.az4DemoTable || len(pool) == 0 {
		return nil
	}
	return &host.TableConfig{
		ID: id, Name: "az-c4 vs az-c4 (FDN)", Seats: 2, Decks: pool,
		Seed: seed, Pace: c.pace, Spectator: vis, Perpetual: c.perpetual,
		Mulligans: c.mulligans, Format: host.FormatConstructed, BotPolicy: azc4.Policy,
		BotAutoPayMana: c.botAutoPayMana, AutoMana: c.autoMana,
	}
}

// reservedPolicyNames is every policy with a reserved deck pool. Draft
// events deal their own decks, so they never offer one.
func (c config) reservedPolicyNames() []string {
	var out []string
	for p := range c.policyPools {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
