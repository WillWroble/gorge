// Package bots provides the closed registry of policies available to callers.
package bots

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/seat"
)

type Tier string

const (
	Production           Tier = "production"
	Experimental         Tier = "experimental"
	Default                   = "bot"
	BotPolicy                 = Default
	LethalPressurePolicy      = "lethal-pressure"
	CastProfilePolicy         = "cast-profile"
)

type Measurement struct {
	Claim, Versus, Setting, Source string
}

type Cost struct {
	MeanMS, P95MS       float64
	Scope, Note, Source string
}

type Info struct {
	Name, Label, Description string
	Tier                     Tier
	Strength                 []Measurement
	Cost                     Cost
	// Env marks a policy whose seat builds a full search engine over every
	// seat's REAL decklist (server/host/botenv.go envData: searchprobe.PublicGame
	// from m.cfg.Decks). "Honest" root redeals the hidden cards, but the
	// deck LIST it redeals from is each seat's true list, so an Env seat
	// knows every seat's real decklist. Operator ruling 2026-10-13: Env
	// therefore never seats against a human, and on a bot-only table only
	// with TableConfig.AllowDecklistBots (enforced by host's table
	// validation and gorged's offered-set gate).
	Env, Search bool
	// ClosedLists marks an Env policy whose every seat closes its
	// opponents' deck lists: the seat is a searchseat.ClosedListSeat whose
	// SearchBelief is never nil, so the host hands it a closed Setup and a
	// closed honest root (server/host/botenv.go envData, searchseat.SeatSetup) and
	// it never reads an opponent's real list or real hidden cards. Such a
	// policy does not read decklists (ReadsDecklists is false) and may seat
	// against a human without AllowDecklistBots. The declaration is not
	// trusted on its own: at a table with a human the host checks every
	// built seat at match start and refuses an open one, and envData
	// refuses an open root at every decision (server/host/closedlist.go).
	ClosedLists bool
	Formats     []string
	MaxSeats    int
	Caretaker   string
	// NeedsAutoPay marks a policy that plays through the engine's automatic
	// mana payment plans and has no manual-tap mode (its factory refuses
	// Options.AutoPayMana == false). The host refuses a table that names it
	// without payment plans when the table is registered, rather than letting
	// the factory's refusal crash the match at its first seat.
	NeedsAutoPay bool
}

type Options struct {
	Seed              uint64
	AutoPayMana       bool
	SearchParallelism int
	Deps              Deps
	// DecisionDeadlineMS is the per-decision wall-clock budget a HOSTED
	// search seat declares (server/host/bot_policy.go's hostedDecisionDeadlineMS;
	// bench and training factories leave it zero and stay unbounded for
	// reproducible measurement). The host arms it as the deadline of the
	// context it hands DecideEnv (host.decisionCtx -- this package may not
	// import time, internal/archtest), so a search that outlives the budget
	// is aborted by the search's own bail-out and the policy's non-searched
	// fallback answers. See bots.BudgetedSeat.
	DecisionDeadlineMS int
	// CommonSense opts the `bot` and `sb-tactical` factories into the
	// bots/sanity guard (sanity.Wrap): hostile removal the engine says can
	// only hit the seat's own permanents or player is dropped from the
	// option list the policy sees, and the London mulligan ask is answered
	// by botpolicy.MulliganConstructed instead of the policy's 1/3 coin.
	// Default false: the returned seat is then
	// exactly the unwrapped policy, so no head, name or training run moves.
	// No other policy reads it.
	CommonSense bool
}

type Deps struct{ Cards *cards.Registry }
type Factory func(Options) (seat.Seat, error)

// BudgetedSeat is a seat carrying a per-decision wall-clock budget
// (Options.DecisionDeadlineMS). The host arms it on the context it hands
// DecideEnv; this package may not import time (internal/archtest), so the
// budget is plain milliseconds and the host -- the only tier that may read
// the clock -- turns it into the context deadline. A search aborted by it
// plays the policy's own non-searched fallback (azmcts Search's and
// sbsearch's bail-outs); it never fails the decision.
type BudgetedSeat interface {
	DecisionBudgetMS() int
}
type Entry struct {
	Info
	New Factory
}

var entries = map[string]Entry{}
var validName = regexp.MustCompile(`^[a-z0-9-]+$`)

func Register(e Entry) {
	if !validName.MatchString(e.Name) || e.New == nil || (e.Tier != Production && e.Tier != Experimental) || (e.ClosedLists && !e.Env) {
		panic(fmt.Sprintf("bots: invalid entry %q", e.Name))
	}
	if _, exists := entries[e.Name]; exists {
		panic("bots: duplicate entry " + e.Name)
	}
	entries[e.Name] = e
}

func Lookup(name string) (Entry, bool) { e, ok := entries[name]; return e, ok }

func Normalize(name string) (string, error) {
	if name == "" {
		name = Default
	}
	if _, ok := Lookup(name); ok {
		return name, nil
	}
	return "", fmt.Errorf("bots: unknown bot policy %q (known: %s)", name, joinNames())
}

func New(name string, o Options) (seat.Seat, error) {
	name, err := Normalize(name)
	if err != nil {
		return nil, err
	}
	return entries[name].New(o)
}

func Names() []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReadsDecklists reports whether the named policy builds a search engine
// over every seat's REAL decklist (Info.Env without Info.ClosedLists;
// server/host/botenv.go's envData). The single discriminator for the operator
// ruling of 2026-10-13: such a policy seats only on a bot-only table that
// set TableConfig's AllowDecklistBots, and never against a human. A
// closed-list policy (az-c4) is an Env policy that does NOT read decklists.
// An unknown name is false.
func ReadsDecklists(name string) bool {
	e, ok := Lookup(name)
	return ok && e.ReadsDecklists()
}

// ReadsDecklists is the Info form of the package function: Env and not
// ClosedLists.
func (i Info) ReadsDecklists() bool { return i.Env && !i.ClosedLists }

func Entries() []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tier != out[j].Tier {
			return out[i].Tier == Production
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func Validate() error {
	for _, e := range Entries() {
		if e.Caretaker == "" {
			continue
		}
		caretaker, ok := Lookup(e.Caretaker)
		if !ok {
			return fmt.Errorf("bots: %q caretaker %q is not registered", e.Name, e.Caretaker)
		}
		if caretaker.Env {
			return fmt.Errorf("bots: %q caretaker %q requires Env", e.Name, e.Caretaker)
		}
	}
	return nil
}

func joinNames() string {
	names := Names()
	ordered := make([]string, 0, len(names))
	for _, preferred := range []string{Default, LethalPressurePolicy} {
		if _, ok := Lookup(preferred); ok {
			ordered = append(ordered, preferred)
		}
	}
	for _, name := range names {
		if name != Default && name != LethalPressurePolicy {
			ordered = append(ordered, name)
		}
	}
	return strings.Join(ordered, ", ")
}
