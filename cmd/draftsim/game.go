package main

import (
	"fmt"

	"github.com/adams-shaun/gorge/botpolicy"
	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/event"
	"github.com/adams-shaun/gorge/internal/bench"
	"github.com/adams-shaun/gorge/limited/build"
	"github.com/adams-shaun/gorge/replay"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/seat"
)

// The game watchdogs, the same props cmd/dzgorge's play lane uses: a
// limited game that reaches either cap is recorded as a watchdog draw in
// its series (a GameEnded with no winner), while a livelock or a recovered
// engine panic is a hard error — an engine bug, not a slow game.
const (
	gameMaxTurns   = 150
	gameMaxIntents = 40000
	// draftMulligans is the London mulligan round each seat may take (a
	// 40-card deck with ~17 lands wants one more often than constructed);
	// the keep/mulligan answers come from botpolicy.MulliganLands.
	draftMulligans = 2
)

// playGame plays one Bo1 game between seats a and b through the rules
// engine the way cmd/mtgsim does, and with verify set replays the chain
// afterwards. It returns the game's record with its chain head; the
// winner is the event seat index (a drives player 0, b player 1), nil for
// a draw or a watchdog stall.
func playGame(r *runner, eventSeed uint64, round, a, b, game int, deckA, deckB []*cards.Card, verify bool) (gameRecord, error) {
	seed := gameSeed(eventSeed, round, a, b, game)
	oc, eng, err := driveGame(r, seed, a, b, deckA, deckB)
	if err != nil {
		return gameRecord{}, err
	}
	rec := gameRecord{
		head:     eng.L.Head(),
		turns:    int(oc.Turns),
		intents:  oc.Intents,
		verified: verify,
	}
	switch {
	case bench.IsAbort(oc.StallOn):
		return gameRecord{}, fmt.Errorf("seed %d: engine aborted (%s): %s", seed, oc.StallOn, firstLine(oc.Livelock))
	case oc.StallOn != "":
		rec.stall = oc.StallOn
	case oc.Draw:
		// winner stays nil: a drawn game
	default:
		w := a
		if oc.WinnerSeat == 1 {
			w = b
		}
		rec.winner = &w
	}
	if verify && rec.stall == "" {
		if _, err := replay.Replay(eng.L, gameConfig(r, seed, a, b, deckA, deckB)); err != nil {
			return gameRecord{}, fmt.Errorf("seed %d: replay: %w", seed, err)
		}
	}
	return rec, nil
}

// replayGameHead re-plays one logged game from the log-rebuilt event's
// saved decks and returns the chain head it produces, for -verify's
// head-for-head comparison.
func replayGameHead(r *runner, ev *event.Event, round, a, b, game int) (string, error) {
	deckA, err := resolveSavedDeck(r, ev, a)
	if err != nil {
		return "", fmt.Errorf("seat %d deck: %w", a, err)
	}
	deckB, err := resolveSavedDeck(r, ev, b)
	if err != nil {
		return "", fmt.Errorf("seat %d deck: %w", b, err)
	}
	seed := gameSeed(ev.Seed(), round, a, b, game)
	oc, eng, err := driveGame(r, seed, a, b, deckA, deckB)
	if err != nil {
		return "", err
	}
	if oc.StallOn != "" {
		return "", fmt.Errorf("seed %d: replay ended as a %s stall", seed, oc.StallOn)
	}
	return eng.L.Head(), nil
}

// resolveSavedDeck turns the event's saved deck revision into the engine's
// card pointers, the same path the live run used (SavedDeck → ToDeckFile →
// Resolve), so a replayed game reads the same 40 cards in the same order.
func resolveSavedDeck(r *runner, ev *event.Event, seatIdx int) ([]*cards.Card, error) {
	d, ok := ev.SavedDeck(seatIdx)
	if !ok {
		return nil, fmt.Errorf("seat %d has no saved deck in the log", seatIdx)
	}
	pool := ev.Pool(seatIdx)
	f := build.ToDeckFile(pool, d, fmt.Sprintf("p%d %s draft", seatIdx, r.spec.Code))
	return f.Resolve(r.reg)
}

// driveGame builds the two seats and the engine config for one game and
// drives it to completion (or to a watchdog cap) on the caller's own
// goroutine, exactly cmd/mtgsim's drive loop's contract.
func driveGame(r *runner, seed uint64, a, b int, deckA, deckB []*cards.Card) (bench.Outcome, *rules.Engine, error) {
	seats := make([]seat.Seat, 2)
	for i, p := range []int{a, b} {
		st, err := bots.New(r.players[p], bots.Options{
			Seed:        seed ^ uint64(i+1),
			AutoPayMana: true,
			Deps:        bots.Deps{Cards: r.reg},
		})
		if err != nil {
			return bench.Outcome{}, nil, fmt.Errorf("seat %d policy %q: %w", p, r.players[p], err)
		}
		// The limited mulligan: the plain policies all build on *seat.Bot,
		// whose London rule is opt-in per seat. A policy that some day
		// wraps its bot loses the heuristic and keeps the default coin.
		if bot, ok := st.(*seat.Bot); ok {
			bot.WithMulligan(botpolicy.MulliganLands)
		}
		seats[i] = st
	}
	oc, eng, err := bench.PlayGame(gameConfig(r, seed, a, b, deckA, deckB), seats, gameMaxTurns, gameMaxIntents, bench.Hooks{})
	if err != nil {
		return bench.Outcome{}, nil, fmt.Errorf("seed %d: %w", seed, err)
	}
	return oc, eng, nil
}

// gameConfig is one game's engine config: the two decks, the derived seed
// and the name universe the engine's NameCard asks may need.
func gameConfig(r *runner, seed uint64, a, b int, deckA, deckB []*cards.Card) rules.Config {
	return rules.Config{
		Seed:         seed,
		Names:        []string{fmt.Sprintf("p%d", a), fmt.Sprintf("p%d", b)},
		Decks:        [][]*cards.Card{deckA, deckB},
		Tokens:       r.reg.Tokens,
		NameUniverse: r.reg.Universe(),
		Mulligans:    draftMulligans,
	}
}

// firstLine cuts a diagnostic to its first line, for the game-abort error.
func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
