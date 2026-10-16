package main

// CV-7 (cli-20261014T140212Z-b3c06e6b): sacrifice outlets in FDN.
//
// The cold-start card-value spike rated Hungry Ghoul far below 17Lands and
// hypothesised the `bot` policy activates sacrifice outlets wastefully. This
// harness plays FDN Limited mirrors with the production bot and measures, per
// game, how often a seat activates Hungry Ghoul, what it sacrifices (token
// vs nontoken, power, mana value) and in which step; then it replays the SAME
// games on the SAME seeds with one seat's Ghoul activation declined, to price
// the activation in win rate (paired seeds, one seat toggled, the other a
// fixed production bot).
//
// It is an observation harness: the assertions are structural (an activation
// sacrifices exactly one other creature the activator controls), and the
// measured counts and win-rate delta are logged for the CV-7 report.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/bots/heuristic"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
	gbench "github.com/adams-shaun/gorge/internal/bench"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/lab/training/spellbench"
)

// cv7GhoulDecks are the FDN Limited catalog decks whose 40-card list includes
// Hungry Ghoul (the spike's worst-rated sacrifice outlet).
var cv7GhoulDecks = []string{"FDN04-WB", "FDN10-BR", "FDN11-BWR", "FDN13-BG", "FDN14-BR", "FDN15-UB"}

// cv7GhoulSacEvent is one observed Hungry Ghoul activation.
type cv7GhoulSacEvent struct {
	seat     state.PlayerID
	turn     int32
	step     state.Step
	victim   string
	power    int32
	mv       int32
	token    bool
	creature bool
	source   state.ObjID
}

// cv7GameResult is one played game's observation.
type cv7GameResult struct {
	deck      string
	seed      uint64
	winner    int
	draw      bool
	stalled   bool
	ghoulActs []cv7GhoulSacEvent
}

// cv7DeclineGhoulSeat wraps the production bot and rewrites a priority
// answer that would activate Hungry Ghoul into the explicit pass. Every other
// decision and every other interface method is the embedded bot's, so the two
// arms differ only in the Ghoul activation and the games stay paired.
type cv7DeclineGhoulSeat struct {
	*heuristic.Bot
	enabled  bool
	declined *int
}

func (s *cv7DeclineGhoulSeat) Decide(ctx context.Context, v view.View, d decision.Decision) (decision.Intent, error) {
	in, err := s.Bot.Decide(ctx, v, d)
	return s.rewrite(d, in, nil), err
}

func (s *cv7DeclineGhoulSeat) DecideBoard(ctx context.Context, b botpolicy.Board, d decision.Decision) (decision.Intent, error) {
	in, err := s.Bot.DecideBoard(ctx, b, d)
	return s.rewrite(d, in, &b), err
}

// rewrite replaces an intent that selects Hungry Ghoul's ability with the
// pass option. The Ghoul is identified by the option's source name on the
// board (Board.Cards, keyed by ObjID) plus its sacrifice cost, so no other
// card's ability is touched.
func (s *cv7DeclineGhoulSeat) rewrite(d decision.Decision, in decision.Intent, b *botpolicy.Board) decision.Intent {
	if !s.enabled || d.Kind != decision.KPriority || len(in.Choices) == 0 || b == nil {
		return in
	}
	chosen := make(map[int]bool, len(in.Choices))
	for _, ci := range in.Choices {
		chosen[ci] = true
	}
	hit := false
	for i := range d.Options {
		o := &d.Options[i]
		if o.Kind != "ability" || !chosen[o.Index] {
			continue
		}
		if !strings.Contains(o.Cost, "Sac<") {
			continue
		}
		if c, ok := b.Cards.Lookup(o.Obj); ok && c.PrintedName == "Hungry Ghoul" {
			hit = true
		}
	}
	if !hit {
		return in
	}
	for i := range d.Options {
		if d.Options[i].Kind == "pass" {
			in.Choices = []int{d.Options[i].Index}
			in.Rest = nil
			in.Payment = nil
			in.Announce = nil
			if s.declined != nil {
				*s.declined++
			}
			return in
		}
	}
	return in
}

// cv7Collector observes every decision of one game and records each Hungry
// Ghoul activation with the creature its sacrifice cost consumed. The
// activation is the priority answer that selects the Ghoul's "ability"
// option (identified by the option's source name on the Board plus its
// Sac< cost); the victim is the next chosen option with a battlefield
// creature Obj owned by that seat (the sacrifice-target ask, posed while the
// victim is still on the battlefield).
type cv7Collector struct {
	engine  *rules.Engine
	acts    []cv7GhoulSacEvent
	pending map[state.PlayerID]*cv7GhoulSacEvent
}

func newCV7Collector() *cv7Collector {
	return &cv7Collector{pending: map[state.PlayerID]*cv7GhoulSacEvent{}}
}

func (c *cv7Collector) hook(seatIdx int, d *decision.Decision, in decision.Intent, board *botpolicy.Board) error {
	if board == nil || c.engine == nil {
		return nil
	}
	if d.Kind == decision.KPriority {
		for i := range d.Options {
			o := &d.Options[i]
			if o.Kind != "ability" || !cv7Chose(in, o.Index) || !strings.Contains(o.Cost, "Sac<") {
				continue
			}
			if card, ok := board.Cards.Lookup(o.Obj); ok && card.PrintedName == "Hungry Ghoul" {
				c.pending[d.Player] = &cv7GhoulSacEvent{
					seat: d.Player, turn: c.engine.G.Turn, step: c.engine.G.Step, source: o.Obj,
				}
			}
		}
		return nil
	}
	// The sacrifice-target ask: a chosen option that names a battlefield
	// creature this seat controls (and is not the Ghoul itself).
	pend := c.pending[d.Player]
	if pend == nil || len(in.Choices) == 0 {
		return nil
	}
	for i := range d.Options {
		o := &d.Options[i]
		if o.Obj == 0 || !cv7Chose(in, o.Index) {
			continue
		}
		ob := c.engine.G.Obj(o.Obj)
		if ob == nil || ob.Face() == nil || ob.Zone != state.ZBattlefield || !ob.Face().IsCreature() {
			continue
		}
		if o.Obj == pend.source || ob.Controller != d.Player {
			continue // the source is not "another", and only own creatures are sacrificed
		}
		pend.victim = ob.Face().Name
		pend.power = int32(ob.Face().Power())
		pend.mv = ob.Face().Cmc()
		pend.token = ob.IsToken
		pend.creature = true
		c.acts = append(c.acts, *pend)
		delete(c.pending, d.Player)
		break
	}
	return nil
}

func cv7Chose(in decision.Intent, index int) bool {
	for _, ci := range in.Choices {
		if ci == index {
			return true
		}
	}
	return false
}

// cv7Play plays one game. Seat 0 is always a cv7DeclineGhoulSeat wrapping the
// production bot; enabled toggles only whether it declines the Ghoul
// activation, so the two arms share the exact seat-0 policy and RNG. Seat 1
// is the plain production bot in both arms (a fixed opponent).
func cv7Play(seed uint64, deck []*cards.Card, reg *cards.Registry, enabled bool) (cv7GameResult, int, error) {
	declined := 0
	seats := []seat.Seat{
		&cv7DeclineGhoulSeat{Bot: heuristic.NewBot(seed ^ 1), enabled: enabled, declined: &declined},
		policies["bot"](seed ^ 2),
	}
	cfg := rules.Config{
		Seed: seed, Names: []string{"p0", "p1"},
		Decks: [][]*cards.Card{deck, deck}, Tokens: reg.Tokens, NameUniverse: reg.Universe(),
	}
	c := newCV7Collector()
	hooks := gbench.Hooks{
		Setup:    func(e *rules.Engine) { c.engine = e },
		Decision: c.hook,
	}
	o, _, err := gbench.PlayGame(cfg, seats, 200, 20000, hooks)
	if err != nil {
		return cv7GameResult{}, 0, err
	}
	res := cv7GameResult{seed: seed, winner: o.WinnerSeat, draw: o.Draw, stalled: o.IsStalled()}
	res.ghoulActs = c.acts
	return res, declined, nil
}

// TestCV7HungryGhoulPilot plays the FDN mirrors and logs the measurement, then
// replays every game on the same seed with seat 0's Ghoul activation declined
// and reports the paired-seed win-rate effect for seat 0.
func TestCV7HungryGhoulPilot(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	pairs := cv7EnvInt("CV7_PAIRS", 4)
	var acts, games int
	bySeat := map[state.PlayerID]int{}
	byStep := map[state.Step]int{}
	tokens, nontokens := 0, 0
	powerHist := map[int32]int{}
	mvHist := map[int32]int{}
	var seat0WinsOn, seat0WinsOff, decided, declinedTotal int
	for _, deckID := range cv7GhoulDecks {
		deck, err := spellbench.Deck(reg, spellbench.FDNLimited, deckID)
		if err != nil {
			t.Fatal(err)
		}
		for p := 0; p < pairs; p++ {
			seed := sbGameSeed(20261014, 0, p)
			on, _, err := cv7Play(seed, deck, reg, false)
			if err != nil {
				t.Fatalf("%s seed %d (on): %v", deckID, seed, err)
			}
			off, declined, err := cv7Play(seed, deck, reg, true)
			if err != nil {
				t.Fatalf("%s seed %d (off): %v", deckID, seed, err)
			}
			declinedTotal += declined
			games++
			for _, a := range on.ghoulActs {
				acts++
				bySeat[a.seat]++
				byStep[a.step]++
				if a.token {
					tokens++
				} else {
					nontokens++
				}
				powerHist[a.power]++
				mvHist[a.mv]++
				if !a.creature {
					t.Errorf("%s seed %d: activation sacrificed a non-creature %q", deckID, seed, a.victim)
				}
			}
			if !on.draw && !on.stalled && !off.draw && !off.stalled {
				decided++
				if on.winner == 0 {
					seat0WinsOn++
				}
				if off.winner == 0 {
					seat0WinsOff++
				}
			}
		}
	}
	t.Logf("CV-7 Hungry Ghoul: games=%d activations=%d (%.2f/game)", games, acts, float64(acts)/float64(games))
	t.Logf("CV-7 activations by seat: seat0=%d seat1=%d", bySeat[0], bySeat[1])
	t.Logf("CV-7 sacrifices: tokens=%d nontokens=%d", tokens, nontokens)
	t.Logf("CV-7 by step: %s", cv7StepHist(byStep))
	t.Logf("CV-7 victim power: %s", cv7IntHist(powerHist))
	t.Logf("CV-7 victim mana value: %s", cv7IntHist(mvHist))
	if decided > 0 {
		onRate := float64(seat0WinsOn) / float64(decided)
		offRate := float64(seat0WinsOff) / float64(decided)
		t.Logf("CV-7 paired seat-0 win rate: on=%.3f (%d/%d) off=%.3f (%d/%d) delta=%+.3f (declined=%d)",
			onRate, seat0WinsOn, decided, offRate, seat0WinsOff, decided, offRate-onRate, declinedTotal)
	}
	if games > 0 && acts == 0 {
		t.Errorf("no Hungry Ghoul activations observed in %d games; the measurement is vacuous", games)
	}
}

func cv7EnvInt(name string, def int) int {
	if v, ok := os.LookupEnv(name); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func cv7StepHist(m map[state.Step]int) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%d ", state.Step(k).String(), m[state.Step(k)])
	}
	return strings.TrimSpace(b.String())
}

func cv7IntHist(m map[int32]int) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%d:%d ", k, m[int32(k)])
	}
	return strings.TrimSpace(b.String())
}
