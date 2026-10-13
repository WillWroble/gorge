package eventhost

import (
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/adams-shaun/gorge/event"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/view"
)

// gameSeedSalt is draftsim's per-game seed salt: the fixed top bits of the
// PCG stream key every game table is seeded from (cmd/draftsim/event.go).
const gameSeedSalt = 0x6A111E

// gameSeed derives one game table's seed from the event seed. E1 is Bo1
// only — game is always 1 here; E3 adds a games-played accessor and passes
// the real game number through.
func gameSeed(seed uint64, round, a, b, game, attempt int) uint64 {
	salt := gameSeedSalt<<32 | uint64(round)<<24 | uint64(a)<<20 |
		uint64(b)<<16 | uint64(game)<<8 | uint64(attempt)
	return rand.New(rand.NewPCG(seed, salt)).Uint64()
}

// gameTableID names one game attempt's OnDemand host table. The attempt
// suffix makes every attempt's table unique, so a replayed game never
// reuses a table id whose match rows are still served.
func gameTableID(ev string, round, a, b, game, attempt int) host.TableID {
	return host.TableID(fmt.Sprintf("%s-r%d-s%d%d-g%d-a%d", ev, round, a, b, game, attempt))
}

// startGame logs and launches one series' next game attempt on its own
// OnDemand table. The caller holds r.mu. The GameStarted entry is logged
// BEFORE the table exists, so a crash between the log line and AddTable
// leaves an in-flight attempt that resume aborts — and a table id is never
// reused, because the next attempt takes the next attempt number. Any
// failure after the log line aborts the attempt, blocks the event (no more
// games start until the process restarts; resume clears blocked) and
// returns the error.
func (r *run) startGame(sp event.SeriesSpec) error {
	o := r.h.opts
	a, b := sp.A, sp.B
	attempt := r.ev.NextAttempt(a, b)
	id := gameTableID(r.meta.ID, sp.Round, a, b, 1, attempt)
	seed := gameSeed(r.meta.Seed, sp.Round, a, b, 1, attempt)
	if err := r.apply(event.Entry{
		Kind: event.KindGameStarted, Round: sp.Round, A: a, B: b,
		Game: 1, Attempt: attempt, Table: string(id), GameSeed: seed,
	}); err != nil {
		return err
	}
	fail := func(err error) error {
		_ = r.apply(event.Entry{
			Kind: event.KindGameAborted, Round: sp.Round, A: a, B: b,
			Game: 1, Attempt: attempt, Reason: "start: " + err.Error(),
		})
		r.h.mu.Lock()
		delete(r.h.tables, id)
		r.h.mu.Unlock()
		delete(r.tokens, id)
		r.blocked = err.Error()
		return err
	}
	var humanSlots []int
	for i, s := range []int{a, b} {
		if r.meta.Seats[s].Kind == "human" {
			humanSlots = append(humanSlots, i)
		}
	}
	for _, slot := range humanSlots {
		if o.MintSeat == nil {
			return fail(errors.New("eventhost: Options.MintSeat is required to seat a human"))
		}
		tok, err := o.MintSeat(id, slot)
		if err != nil {
			return fail(err)
		}
		r.tokens[id] = tok
	}
	r.h.mu.Lock()
	r.h.tables[id] = gameRef{run: r, round: sp.Round, a: a, b: b, game: 1, attempt: attempt}
	r.h.mu.Unlock()
	cfg := host.TableConfig{
		ID:    id,
		Name:  fmt.Sprintf("%s round %d: %s vs %s", r.meta.ID, sp.Round, r.nameOf(a), r.nameOf(b)),
		Seats: 2,
		// Decks [b, a] + match 1's rotation seats event seat a at table
		// seat 0 and b at seat 1, matching PlayerNames and SeatBotPolicies.
		Decks: []string{
			DeckID(r.meta.ID, b, r.ev.ReadyRev(b)),
			DeckID(r.meta.ID, a, r.ev.ReadyRev(a)),
		},
		Seed:            seed,
		PlayerNames:     []string{r.nameOf(a), r.nameOf(b)},
		SeatBotPolicies: []string{r.policyOrEmpty(a), r.policyOrEmpty(b)},
		Humans:          humanSlots,
		BotPolicy:       o.DefaultPolicy,
		Spectator:       view.Public,
		Format:          host.FormatConstructed,
		Mulligans:       o.Mulligans,
		AutoMana:        o.AutoMana,
		BotAutoPayMana:  o.BotAutoPayMana,
		BotCommonSense:  o.BotCommonSense,
		OnDemand:        true,
		Event:           r.meta.ID,
	}
	if err := o.Registry.AddTable(cfg); err != nil {
		return fail(err)
	}
	if err := o.Registry.Start(id); err != nil {
		return fail(err)
	}
	return nil
}

// nameOf is one seat's display name: a bot's stored name, a human's joined
// name (games never start before every human seat has joined).
func (r *run) nameOf(seat int) string {
	return r.meta.Seats[seat].Name
}

// policyOrEmpty is a seat's game policy, or "" on a human slot (its table
// caretaker follows TableConfig.BotPolicy instead).
func (r *run) policyOrEmpty(seat int) string {
	if r.meta.Seats[seat].Kind == "human" {
		return ""
	}
	return r.meta.Seats[seat].Policy
}

// advanceRounds drives the ROUND(1..3) phases: it flushes buffered game
// results into the log in pairing order, starts every pending series' next
// game attempt, and closes the event once round 3 is over. It is called
// under r.mu and reports whether it applied anything. A blocked event
// starts no more games until the process restarts.
func (r *run) advanceRounds() bool {
	switch r.ev.Phase() {
	case event.RoundPhase(1), event.RoundPhase(2), event.RoundPhase(3):
	default:
		return false
	}
	r.h.mu.Lock()
	closing := r.h.closed
	r.h.mu.Unlock()
	if closing || r.blocked != "" {
		return false
	}
	applied := r.flush()
	for _, sp := range r.ev.PendingSeries() {
		if !r.ev.Started(sp.A, sp.B) {
			if err := r.apply(event.Entry{
				Kind: event.KindSeriesStarted, Round: sp.Round, A: sp.A, B: sp.B, BestOf: 1,
			}); err != nil {
				return applied
			}
			applied = true
		}
		if _, buffered := r.outcomes[[2]int{sp.A, sp.B}]; buffered {
			continue
		}
		live := false
		for _, att := range r.ev.InFlight() {
			if att.A == sp.A && att.B == sp.B {
				live = true
				break
			}
		}
		if live {
			continue
		}
		if err := r.startGame(sp); err != nil {
			return applied
		}
		applied = true
	}
	if r.ev.Phase() == event.RoundPhase(3) && len(r.ev.PendingSeries()) == 0 {
		if err := r.apply(event.Entry{Kind: event.KindEventEnded}); err != nil {
			return applied
		}
		applied = true
	}
	return applied
}

// abortInFlight aborts every game attempt a restart caught in flight, so
// the first advance starts each one again on a fresh table with a fresh
// seed. It is called under r.mu from New, before the first advance.
func (r *run) abortInFlight() {
	for _, att := range r.ev.InFlight() {
		if err := r.apply(event.Entry{
			Kind: event.KindGameAborted, Round: att.Round, A: att.A, B: att.B,
			Game: att.Game, Attempt: att.Attempt, Reason: "server restart",
		}); err != nil {
			return
		}
	}
}
