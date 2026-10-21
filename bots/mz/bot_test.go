package mz_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adams-shaun/gorge/bots"
	_ "github.com/adams-shaun/gorge/bots/all"
	"github.com/adams-shaun/gorge/bots/mz"
	"github.com/adams-shaun/gorge/bots/mz/decks"
	"github.com/adams-shaun/gorge/bots/mz/remote"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/testkit/bench"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestSearchPlaysDeterministically plays the mz bot against gorge's bot in
// both tap modes, twice: every game finishes, and the same seed plays the
// same game (principle 0), with every search finding a legal action.
func TestSearchPlaysDeterministically(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	names := []string{"pauper-faeries", "pauper-cawgates"}
	ds := make([][]*cards.Card, 2)
	for i, n := range names {
		d, err := decks.Load(reg, n)
		if err != nil {
			t.Fatal(err)
		}
		ds[i] = d
	}
	play := func(manual bool) (string, mz.Stats) {
		opt := mz.Defaults()
		opt.Budget, opt.Timeout, opt.PruneDuplicates, opt.ManualTap = 20, time.Hour, true, manual
		bot := mz.New(opt, nil)
		opp, err := bots.New(bots.Default, bots.Options{Seed: 2, AutoPayMana: !manual})
		if err != nil {
			t.Fatal(err)
		}
		cfg := rules.Config{Seed: 1, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
		o, e, err := bench.PlayGame(cfg, []seat.Seat{bot, opp}, 50, 400000, bench.Hooks{})
		if err != nil {
			t.Fatal(err)
		}
		if o.IsStalled() {
			t.Fatalf("manual=%v: game stalled: %s %s", manual, o.StallOn, o.Livelock)
		}
		return e.L.Head(), bot.Stats
	}
	for _, manual := range []bool{false, true} {
		h1, st := play(manual)
		if st.Searches == 0 || st.Sims == 0 {
			t.Fatalf("manual=%v: no search ran: %+v", manual, st)
		}
		if h2, _ := play(manual); h1 != h2 {
			t.Errorf("manual=%v: same seed, different games: %s vs %s", manual, h1, h2)
		}
	}
}

// TestOnlineSearchPlays plays a game through a stand-in model server with
// every policy head, root noise and visit sampling on: MCTSNode2's online
// path (priors from the policy, the value from the network) must finish the
// game. Online search is not deterministic (its evaluations land when they
// arrive), so only completion is checked.
func TestOnlineSearchPlays(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	names := []string{"pauper-faeries", "pauper-cawgates"}
	ds := make([][]*cards.Card, 2)
	for i, n := range names {
		d, err := decks.Load(reg, n)
		if err != nil {
			t.Fatal(err)
		}
		ds[i] = d
	}
	srv := httptest.NewServer(remote.Handler(func(ids []int32) remote.Result {
		r := remote.Result{Binary: []float32{0.2, 0.8}, Value: float32(len(ids)%7)/7 - 0.5}
		for _, id := range ids {
			r.Priority = append(r.Priority, float32(id%13)/13)
		}
		r.Target = r.Priority
		return r
	}))
	defer srv.Close()
	net, err := remote.Dial(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer net.Close()
	opt := mz.Defaults()
	opt.Budget, opt.Timeout = 20, time.Hour
	opt.NoPolicyPriority, opt.NoPolicyTarget, opt.NoPolicyUse, opt.NoPolicyOpponent = false, false, false, false
	opt.NoNoise, opt.DirichletNoiseEps, opt.SelectionTemperature = false, 0.25, 1
	bot := mz.New(opt, net)
	opp, err := bots.New(bots.Default, bots.Options{Seed: 2, AutoPayMana: true})
	if err != nil {
		t.Fatal(err)
	}
	cfg := rules.Config{Seed: 1, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
	o, _, err := bench.PlayGame(cfg, []seat.Seat{bot, opp}, 50, 400000, bench.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if o.IsStalled() {
		t.Fatalf("game stalled: %s %s", o.StallOn, o.Livelock)
	}
	if bot.Stats.Sims == 0 {
		t.Fatal("no search ran")
	}
}
