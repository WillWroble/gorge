// Package pickbot answers a draft's pick and build decisions the way a bot
// seat does: the spec §4.6 Drafter (rating-colour v1) and Builder (best23
// v1). Ratings come from the embedded 17Lands FDN file where it covers the
// set, and from a card-facts heuristic elsewhere. The same functions back
// the human UI's Auto lands and Reset-to-bot-suggestion buttons.
//
// The package is pure: it imports only the limited vocabulary, the deck
// format, the card IR and draft's PickView, and takes its card facts
// through a caller-supplied Lookup so it never loads the corpus itself.
package pickbot

import (
	"math/rand/v2"
	"sort"
	"sync"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/build"
	"github.com/adams-shaun/gorge/limited/draft"
)

// PickView is one seat's view at a pick decision; it is draft.PickView
// spelled the spec §4.6 way, so the pod and the bot share one type.
type PickView = draft.PickView

// Lookup resolves a card name to its card facts. It is supplied by the
// caller (the corpus registry in tests and the host) so this package never
// loads the corpus itself. A nil Lookup, or a name it does not know, means
// "no facts": the card has no colours and no type for the heuristic, which
// is why the FDN rated path — name-keyed — works without it.
type CardLookup func(name string) *cards.Card

// Tuning is every constant the drafter and builder read, so the draftsim
// tuning loop (spec §4.8) turns one knob set.
type Tuning struct {
	// RatingCentre is the win rate a centred rating of zero stands for
	// ((gih_wr - RatingCentre) * RatingScale).
	RatingCentre float64
	// RatingScale converts a win-rate gap to rating points.
	RatingScale float64
	// HeuristicCentre and HeuristicScale put the heuristic's raw points on
	// the same rough centred scale as a rated card.
	HeuristicCentre float64
	HeuristicScale  float64
	// ColourFitMax is the full weight k of the colour term; ColourFitRamp
	// is how many picks of pack 1 ramp k from 0 to it (full by pick
	// ColourFitRamp+1, per the spec's "by pack 1 pick 10").
	ColourFitMax  float64
	ColourFitRamp int
	// ColourlessFit is the colour term a colourless card earns (0.5 of the
	// maximum fit of 1).
	ColourlessFit float64
	// MainSize is how many non-land cards the builder plays, DeckSize the
	// total it fills to with basics.
	MainSize int
	DeckSize int
	// MinCreatures is the creature count the builder prefers; HighValueMV
	// and MaxHighValue cap how many cards of that mana value or more it
	// plays.
	MinCreatures int
	HighValueMV  int
	MaxHighValue int
}

// DefaultTuning is the spec §4.6 default knob set.
var DefaultTuning = Tuning{
	RatingCentre:    0.55,
	RatingScale:     100,
	HeuristicCentre: 2,
	HeuristicScale:  2,
	ColourFitMax:    4,
	ColourFitRamp:   9,
	ColourlessFit:   0.5,
	MainSize:        23,
	DeckSize:        40,
	MinCreatures:    14,
	HighValueMV:     6,
	MaxHighValue:    3,
}

// Drafter picks one card ID out of the pack a PickView offers. r is the
// seat's seeded rng, used only to break score ties.
type Drafter interface {
	Pick(v PickView, r *rand.Rand) string
}

// Builder turns a finished pool into a deck.
type Builder interface {
	Build(set string, pool []limited.CardRef) build.Deck
}

type entry struct {
	Drafter Drafter
	Builder Builder
}

var (
	entriesMu sync.RWMutex
	entries   = map[string]entry{}
)

func init() {
	Register("rating-colour", &RatingColour{}, &Best23{})
	Register("best23", &RatingColour{}, &Best23{})
}

// Register names one drafter/builder pair in the registry, mirroring
// bots.Register. Re-registering a name replaces its entry.
func Register(name string, d Drafter, b Builder) {
	entriesMu.Lock()
	defer entriesMu.Unlock()
	entries[name] = entry{Drafter: d, Builder: b}
}

// Lookup returns the registered drafter and builder pair by name.
func Lookup(name string) (Drafter, Builder, bool) {
	entriesMu.RLock()
	defer entriesMu.RUnlock()
	e, ok := entries[name]
	return e.Drafter, e.Builder, ok
}

// Names lists the registered names, sorted.
func Names() []string {
	entriesMu.RLock()
	defer entriesMu.RUnlock()
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
