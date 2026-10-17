package bench_test

// The Env driver (E5-c5): Hooks.EnvSeats drives a bots.EnvSeat the way the
// host does (server/host/botenv.go envData), so a hosted wrapper is answered with its
// bots.Env instead of being silently driven as a plain seat. These tests pin
// the two halves the host relies on: the branch runs at the seat's WantsEnv
// decisions and never hands the seat the live engine, and it is off by default
// (EnvSeats false is byte-identical to the pre-E5-c5 driver).

import (
	"context"
	"testing"

	"github.com/adams-shaun/gorge/bots/heuristic"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/bots/spellbench"
	"github.com/adams-shaun/gorge/testkit/bench"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

const (
	envTestTurns   = 60
	envTestIntents = 20000
)

// envBox captures the live engine Hooks.Setup hands the game, so a seat can
// assert the Env it is given never points at it.
type envBox struct{ live *rules.Engine }

// envTestSeat is a bots.EnvSeat that stays a heuristic.BoardSeat (it embeds a
// *heuristic.Bot), so its non-priority decisions fall through to the BoardSeat
// branch and an EnvSeats-false run is byte-identical to plain bots. Its
// DecideEnv counts calls and records how many carried an honest root and how
// many (illegally) carried the live engine.
type envTestSeat struct {
	*heuristic.Bot
	box    *envBox
	calls  int
	rooted int
	live   int
}

func (s *envTestSeat) WantsEnv(d *decision.Decision) bool { return d.Kind == decision.KPriority }

func (s *envTestSeat) DecideEnv(ctx context.Context, env bots.Env, d decision.Decision) (decision.Intent, error) {
	s.calls++
	if env.Search.Engine != nil {
		s.rooted++
		if env.Search.Engine == s.box.live {
			s.live++
		}
	}
	return s.Bot.DecideBoard(ctx, env.Board, d)
}

// fdnPair loads the first two FDN Limited catalog decks.
func fdnPair(t *testing.T, reg *cards.Registry) ([]*cards.Card, []*cards.Card) {
	t.Helper()
	cat, err := spellbench.CatalogByID("fdn")
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Pool) < 2 {
		t.Fatalf("fdn catalog has %d decks, want >= 2", len(cat.Pool))
	}
	a, err := spellbench.Deck(reg, cat.Dir, cat.Pool[0])
	if err != nil {
		t.Fatal(err)
	}
	b, err := spellbench.Deck(reg, cat.Dir, cat.Pool[1])
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

func envCfg(seed uint64, reg *cards.Registry, a, b []*cards.Card) rules.Config {
	return rules.Config{
		Seed: seed, Names: []string{"p0", "p1"},
		Decks: [][]*cards.Card{a, b}, Tokens: reg.Tokens, NameUniverse: reg.Universe(),
	}
}

// TestPlayGameDrivesEnvSeats: with EnvSeats true the EnvSeat's DecideEnv runs
// at its priority decisions, is handed an honest root that is never the live
// engine, and the game finishes; with EnvSeats false it never runs and the
// game is byte-identical to a plain bot-vs-bot run of the same wrapped bot.
func TestPlayGameDrivesEnvSeats(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	deckA, deckB := fdnPair(t, reg)
	const seed = 7717

	box := &envBox{}
	env := &envTestSeat{Bot: heuristic.NewBot(seed ^ 1), box: box}
	o1, _, err := bench.PlayGame(envCfg(seed, reg, deckA, deckB),
		[]seat.Seat{env, heuristic.NewBot(seed ^ 2)}, envTestTurns, envTestIntents,
		bench.Hooks{EnvSeats: true, Setup: func(e *rules.Engine) { box.live = e }})
	if err != nil {
		t.Fatalf("EnvSeats run: %v", err)
	}
	if o1.IsStalled() {
		t.Fatalf("EnvSeats run stalled on %s", o1.StallOn)
	}
	if env.calls == 0 {
		t.Fatal("DecideEnv was never called with EnvSeats true; the Env driver did not run")
	}
	if env.rooted == 0 {
		t.Fatal("no DecideEnv call carried a non-nil honest root; the Env build did not run")
	}
	if env.live != 0 {
		t.Fatalf("DecideEnv saw the live engine %d times; an EnvSeat must never see it", env.live)
	}

	box2 := &envBox{}
	env2 := &envTestSeat{Bot: heuristic.NewBot(seed ^ 1), box: box2}
	_, off, err := bench.PlayGame(envCfg(seed, reg, deckA, deckB),
		[]seat.Seat{env2, heuristic.NewBot(seed ^ 2)}, envTestTurns, envTestIntents,
		bench.Hooks{EnvSeats: false, Setup: func(e *rules.Engine) { box2.live = e }})
	if err != nil {
		t.Fatalf("EnvSeats-false run: %v", err)
	}
	if env2.calls != 0 {
		t.Fatalf("DecideEnv ran %d times with EnvSeats false", env2.calls)
	}
	_, plain, err := bench.PlayGame(envCfg(seed, reg, deckA, deckB),
		[]seat.Seat{heuristic.NewBot(seed ^ 1), heuristic.NewBot(seed ^ 2)}, envTestTurns, envTestIntents,
		bench.Hooks{})
	if err != nil {
		t.Fatalf("plain run: %v", err)
	}
	if off.L.Head() != plain.L.Head() {
		t.Fatalf("EnvSeats false changed the game: head %s != plain head %s", off.L.Head(), plain.L.Head())
	}
}

// TestPlayGameEnvSeatsIsDeterministic: two EnvSeats runs of the same game give
// the same chain head.
func TestPlayGameEnvSeatsIsDeterministic(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	deckA, deckB := fdnPair(t, reg)
	const seed = 8823

	play := func() string {
		box := &envBox{}
		env := &envTestSeat{Bot: heuristic.NewBot(seed ^ 1), box: box}
		_, e, err := bench.PlayGame(envCfg(seed, reg, deckA, deckB),
			[]seat.Seat{env, heuristic.NewBot(seed ^ 2)}, envTestTurns, envTestIntents,
			bench.Hooks{EnvSeats: true, Setup: func(e *rules.Engine) { box.live = e }})
		if err != nil {
			t.Fatalf("EnvSeats run: %v", err)
		}
		if e == nil {
			t.Fatal("no engine")
		}
		// Precondition: the Env driver actually ran, so "same head" compares
		// two real EnvSeats games, not two plain-bot games.
		if env.calls == 0 {
			t.Fatal("DecideEnv was never called; the Env driver did not run")
		}
		return e.L.Head()
	}
	h1, h2 := play(), play()
	if h1 != h2 {
		t.Fatalf("same seed, different heads: %s vs %s", h1, h2)
	}
}
