package host

// az-c4 (docs/superpowers/reports/2026-10-14-az-c4-hosted.md) through the host:
// the closed-list policy that may seat against a human.
//
//   - TestAZC4NeverSeesTheHumansHiddenCards: the privacy proof. A real hosted
//     table with a human seat whose deck (and sideboard) is made of cards
//     that appear nowhere in az-c4's belief corpus or its own list. At every
//     decision the az-c4 seat is asked, everything the host hands it -- the
//     Env's Setup, honest root engine, Board, feed and refusal reason, and the
//     Board and View of the non-searched paths -- is walked for those cards;
//     a card may appear only once the bot seat could have seen it.
//   - TestHostedEnvSeatsIgnoreTheRealHiddenCardsAZC4: the hidden-card swap
//     row every hosted search entry owes (spec 2026-09-28 §5.3).
//   - TestDecklistGateAtHumanTables: open-list Env policies are still refused
//     at a human table; az-c4 is not, and needs no opt-in.
//   - TestClosedListGuard*: the host checks the seats it built, not the
//     declaration.
//
// az-c4 registers only on Enable, once per binary (enableAZC4); every test
// here that walks bots.Entries skips it by name.

import (
	"context"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adams-shaun/gorge/botpolicy"
	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/azc4"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/gonet/gonettest"
	"github.com/adams-shaun/gorge/internal/searchprobe"
	"github.com/adams-shaun/gorge/internal/searchseat"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/protocol"
	"github.com/adams-shaun/gorge/replay"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/seat"
	"github.com/adams-shaun/gorge/state"
	"github.com/adams-shaun/gorge/view"
)

// azc4BeliefDecks is the test belief corpus: the five Foundations repo decks.
var azc4BeliefDecks = []string{
	"foundations-calling-all-angels", "foundations-keen-engineering", "foundations-reign-of-dragons",
	"foundations-tramplesaurus-rex", "foundations-wretched-ranks",
}

var (
	azc4Once   sync.Once
	azc4Belief *searchprobe.Belief
)

// enableAZC4 registers az-c4 over a synthetic network and the test belief,
// once per test binary.
func enableAZC4(t *testing.T) *searchprobe.Belief {
	t.Helper()
	azc4Once.Do(func() {
		reg := testutil.CorpusRegistry(t)
		var decks [][]*cards.Card
		for _, n := range azc4BeliefDecks {
			decks = append(decks, testutil.RepoDeck(t, reg, n))
		}
		b, err := searchprobe.NewBelief(decks)
		if err != nil {
			t.Fatal(err)
		}
		dir, err := os.MkdirTemp("", "azc4-host-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		path, err := gonettest.Write(dir, "net.dzgw")
		if err != nil {
			t.Fatal(err)
		}
		if err := azc4.Enable(path, b); err != nil {
			t.Fatal(err)
		}
		azc4Belief = b
	})
	if _, ok := bots.Lookup(azc4.Policy); !ok || azc4Belief == nil {
		t.Fatal("az-c4 is not registered")
	}
	return azc4Belief
}

func TestHostedEnvSeatsIgnoreTheRealHiddenCardsAZC4(t *testing.T) {
	enableAZC4(t)
	azLeakRow(t, azc4.Policy, func(actor uint64) seat.Seat {
		s, err := bots.New(azc4.Policy, bots.Options{Seed: actor, AutoPayMana: true})
		if err != nil {
			t.Fatalf("az-c4: %v", err)
		}
		return s
	})
}

// privacySpy wraps the real az-c4 seat at the human's opponent slot. Before
// every answer it hands check what the host handed it; it forwards every
// interface the host dispatches on (Env, Board, closed list, budget).
type privacySpy struct {
	inner seat.Seat
	check func(kind string, d *decision.Decision, env *bots.Env, b *botpolicy.Board, v *view.View)
}

var (
	_ bots.EnvSeat              = (*privacySpy)(nil)
	_ seat.BoardSeat            = (*privacySpy)(nil)
	_ searchseat.ClosedListSeat = (*privacySpy)(nil)
)

func (s *privacySpy) Decide(ctx context.Context, v view.View, d decision.Decision) (decision.Intent, error) {
	s.check("view", &d, nil, nil, &v)
	return s.inner.Decide(ctx, v, d)
}

func (s *privacySpy) DecideBoard(ctx context.Context, b botpolicy.Board, d decision.Decision) (decision.Intent, error) {
	s.check("board", &d, nil, &b, nil)
	return s.inner.(seat.BoardSeat).DecideBoard(ctx, b, d)
}

func (s *privacySpy) WantsEnv(d *decision.Decision) bool { return s.inner.(bots.EnvSeat).WantsEnv(d) }

func (s *privacySpy) DecideEnv(ctx context.Context, env bots.Env, d decision.Decision) (decision.Intent, error) {
	s.check("env", &d, &env, &env.Board, &env.View)
	return s.inner.(bots.EnvSeat).DecideEnv(ctx, env, d)
}

func (s *privacySpy) SearchBelief() *searchprobe.Belief {
	return s.inner.(searchseat.ClosedListSeat).SearchBelief()
}

// publicZone is a zone whose cards every seat sees face up.
func publicZone(z state.Zone) bool {
	switch z {
	case state.ZBattlefield, state.ZGraveyard, state.ZExile, state.ZStack, state.ZCommand:
		return true
	}
	return false
}

// TestAZC4NeverSeesTheHumansHiddenCards plays one whole hosted game, az-c4
// (seat 0) against a human (seat 1, answered by the host's own timeout
// caretaker), and proves the human's unrevealed cards never reach the bot.
//
// The human's deck is mono-red goblins and its sideboard the mono-black
// aggro list; "secret" is every card of the two that is in neither the
// belief corpus nor the bot's own deck, so a secret name anywhere in the
// bot's inputs can only have come from the human's real cards.
//
// revealed, the ground truth, is built from the LIVE game independently of
// the feed and the redealer: a secret card counts as revealed once its object
// has been the subject of an event into a public zone or onto the stack, or
// named in a public reveal (a non-secret Note's IDs), or shown in a View the
// bot seat was handed. The assertion, at every decision of the bot seat: the
// secret names found in what the host handed the seat -- the Env's Setup,
// root engine, Board, feed and refusal reason; the Board and View the
// non-searched paths get -- are a subset of revealed. The root engine is
// walked whole, through unexported fields, except the fields named in
// skipFields (each is a residual the report's leak audit names).
func TestAZC4NeverSeesTheHumansHiddenCards(t *testing.T) {
	belief := enableAZC4(t)
	reg := testutil.CorpusRegistry(t)
	botDeck := testutil.RepoDeck(t, reg, "mono-white-equipment")
	humanDeck := testutil.RepoDeck(t, reg, "mono-red-goblins")
	humanSide := testutil.RepoDeck(t, reg, "mono-black-aggro")
	known := map[string]bool{}
	for _, n := range belief.Cards().Names() {
		known[n] = true
	}
	for _, c := range botDeck {
		known[c.Faces[0].Name] = true
	}
	var secret []*cards.Card
	for _, c := range append(append([]*cards.Card(nil), humanDeck...), humanSide...) {
		if !known[c.Faces[0].Name] {
			secret = append(secret, c)
		}
	}
	// Fields the walk does not enter, each a residual of the audit:
	// compiledText is the engine's immutable card-text cache, built at
	// genesis from every configured deck and shared by every clone; it is
	// read only by lookups keyed on a card the engine already holds.
	skipFields := []string{"Engine.compiledText"}
	scanner := newLeakScan(secret, skipFields...)
	if len(scanner.names) < 8 {
		t.Fatalf("only %d secret names (%v): the fixture does not test much", len(scanner.names), scanner.names)
	}
	t.Logf("secret names (%d): %v", len(scanner.names), scanner.names)

	opts := testOptions(t)
	opts.LoadDeck = func(name string) (Deck, error) {
		switch name {
		case "bot":
			return Deck{Name: "bot", Cards: botDeck}, nil
		case "human":
			return Deck{Name: "human", Cards: humanDeck, Sideboard: humanSide}, nil
		}
		return Deck{}, ErrNotFound
	}
	opts.ThinkTimeout = time.Millisecond // the caretaker answers for the human
	opts.BotUnboundedDecisions = true
	opts.BotSearchParallelism = 4
	opts.MaxIntents = 900

	var (
		mu                     sync.Mutex
		r                      *Registry
		tid                    TableID
		revealed               map[string]bool
		evDone                 int
		kinds                  = map[string]int{}
		decisions, envs, roots int
		posed                  int // roots refused at a posed resolution (posedRefusal)
		otherRefusals          []string
		secretsHidden          int // decisions at which some secret card was still hidden in the live game
		leaks                  []string
		walks                  int
		walkTime               time.Duration
	)
	secretName := map[string]bool{}
	for _, n := range scanner.names {
		secretName[n] = true
	}
	nameOf := func(e *rules.Engine, id state.ObjID) string {
		if o := e.G.Obj(id); o != nil && o.Card != nil && len(o.Card.Faces) > 0 {
			return o.Card.Faces[0].Name
		}
		return ""
	}
	// fold advances revealed over the live log since the last call and
	// counts whether any secret card is still hidden.
	fold := func(e *rules.Engine) bool {
		evs := e.L.Events
		for _, ev := range evs[evDone:] {
			if ev.Obj != 0 && (publicZone(ev.To) || ev.Kind == events.PutOnStack) {
				revealed[nameOf(e, ev.Obj)] = true
			}
			if ev.Kind == events.Note && !ev.Secret {
				for _, id := range ev.IDs {
					revealed[nameOf(e, id)] = true
				}
			}
		}
		evDone = len(evs)
		for i := range e.G.Objs {
			o := &e.G.Objs[i]
			if o.Card != nil && publicZone(o.Zone) {
				revealed[nameOf(e, o.ID)] = true
			}
		}
		hidden := false
		for _, z := range []state.Zone{state.ZHand, state.ZLibrary} {
			for _, id := range e.G.Zone(z, 1) {
				if n := nameOf(e, id); secretName[n] && !revealed[n] {
					hidden = true
				}
			}
		}
		return hidden
	}
	viewNames := func(v *view.View) {
		if v == nil {
			return
		}
		vs := newLeakScan(secret)
		for n := range vs.scan("view", v) {
			revealed[n] = true
		}
	}
	check := func(kind string, d *decision.Decision, env *bots.Env, b *botpolicy.Board, v *view.View) {
		mu.Lock()
		defer mu.Unlock()
		r.mu.RLock()
		tbl := r.tables[tid]
		r.mu.RUnlock()
		tbl.mu.RLock()
		m := tbl.cur
		tbl.mu.RUnlock()
		if m == nil {
			t.Error("the bot was asked with no live match")
			return
		}
		m.mu.RLock()
		if fold(m.e) {
			secretsHidden++
		}
		m.mu.RUnlock()
		viewNames(v)
		decisions++
		kinds[kind+":"+string(d.Kind)]++
		inputs := map[string]any{}
		if b != nil {
			inputs["board"] = b
		}
		if env != nil {
			envs++
			if env.Search.Engine != nil {
				roots++
			} else if env.RootRefused == posedRefusal {
				posed++
			} else {
				otherRefusals = append(otherRefusals, "seq "+itoa(int(d.Seq))+" "+string(d.Kind)+": "+env.RootRefused)
			}
			inputs["setup"] = env.Search.Setup
			inputs["root"] = env.Search.Engine
			inputs["searchBoard"] = env.Search.Board
			inputs["feed"] = env.Search.Feed
			inputs["refused"] = env.RootRefused
			if setup := env.Search.Setup; setup.Belief == nil || len(setup.Decks) != 2 || setup.Decks[1] != nil || setup.Decks[0] == nil {
				t.Errorf("seq %d: the Setup is not closed (belief %v)", d.Seq, setup.Belief != nil)
			}
		}
		start := time.Now()
		for label, in := range inputs {
			for name, paths := range scanner.scan(label, in) {
				if !revealed[name] {
					leaks = append(leaks, kind+" seq "+itoa(int(d.Seq))+": "+name+" at "+strings.Join(paths, " | "))
				}
			}
		}
		walks++
		walkTime += time.Since(start)
	}
	opts.Seats = func(names []string, seed uint64) []seat.Seat {
		inner, err := bots.New(azc4.Policy, bots.Options{Seed: seed ^ 1, AutoPayMana: true, SearchParallelism: 4})
		if err != nil {
			panic(err)
		}
		return []seat.Seat{&privacySpy{inner: inner, check: check}, seat.NewBot(seed ^ 2)}
	}
	var err error
	r, err = New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	// Three games from three seeds, each its own single-shot human table,
	// with the London mulligan on so the non-searched Board path is asked
	// too.
	for g, seed := range []uint64{20261014, 20261015, 20261016} {
		mu.Lock()
		tid, revealed, evDone = TableID("g"+itoa(g+1)), map[string]bool{}, 0
		mu.Unlock()
		cfg := TableConfig{ID: tid, Name: "az-c4 vs a human", Seats: 2,
			Decks: []string{"human", "bot"}, // match 1 deals seat i Decks[(i+1)%2]
			Seed:  seed, Spectator: view.Public, Humans: []int{1}, BotPolicy: azc4.Policy,
			AutoMana: true, BotAutoPayMana: true, Mulligans: 2}
		if err := r.AddTable(cfg); err != nil {
			t.Fatalf("AddTable refused az-c4 at a human table without the decklist opt-in: %v", err)
		}
		if err := r.Start(tid); err != nil {
			t.Fatal(err)
		}
		r.Wait(tid)
		ms, err := r.Matches(tid)
		if err != nil || len(ms) != 1 {
			t.Fatalf("matches %v, %v", ms, err)
		}
		if got := []string{ms[0].Seats[0].Deck, ms[0].Seats[1].Deck}; got[0] != "bot" || got[1] != "human" {
			t.Fatalf("seat decks %v, want the bot deck at the az-c4 seat and the human deck at the human's", got)
		}
		if ms[0].State == protocol.MatchCrashed {
			t.Fatalf("game %d crashed", g+1)
		}
		mu.Lock()
		nRevealed := 0
		for n := range secretName {
			if revealed[n] {
				nRevealed++
			}
		}
		t.Logf("game %d: %s after %d turns (%s); %d of %d secret names revealed by the end", g+1, ms[0].State, ms[0].Turns, ms[0].Result, nRevealed, len(secretName))
		mu.Unlock()
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("bot decisions %d (by path:kind %v), Env %d, roots %d, refused at a posed resolution %d; secrets still hidden at %d decisions; %d walks, %v",
		decisions, kinds, envs, roots, posed, secretsHidden, walks, walkTime)
	for k, n := range scanner.skipped {
		t.Logf("not walked: %s (%d times)", k, n)
	}
	// Every searched decision got a root, bar the posed-resolution refusals:
	// a feed that stopped (as one did whenever the seat answered with a
	// payment plan, before match.go built the plans for the recorded copy)
	// shows up here as "no live observation feed".
	if len(otherRefusals) > 0 {
		t.Errorf("%d roots refused for other reasons, first: %s", len(otherRefusals), otherRefusals[0])
	}
	// Non-vacuity: the bot searched real roots while the human held cards
	// it had not seen.
	if roots < 10 || secretsHidden < 10 {
		t.Fatalf("too little evidence: %d roots, %d decisions with hidden secrets", roots, secretsHidden)
	}
	if len(leaks) > 0 {
		sort.Strings(leaks)
		if len(leaks) > 40 {
			leaks = leaks[:40]
		}
		t.Fatalf("%d leak(s) of the human's unrevealed cards into az-c4's inputs:\n%s", len(leaks), strings.Join(leaks, "\n"))
	}
}

// Open-list Env policies -- az-c3 (enabled here without a belief, as gorged
// serves it), az-redeal and sb-tactical -- are refused at a table with a
// human, with or without the opt-in; az-c4 is accepted there without it, and
// on a bot-only table without it too.
func TestDecklistGateAtHumanTables(t *testing.T) {
	enableAZC3(t)
	enableAZC4(t)
	table := func(policy string, humans []int, optIn bool) TableConfig {
		return TableConfig{ID: "t1", Seats: 2, Decks: []string{"a", "b"}, Seed: 1, Spectator: view.Public,
			Humans: humans, BotPolicy: policy, AutoMana: true, BotAutoPayMana: true, AllowDecklistBots: optIn}
	}
	for _, policy := range []string{azc3.Policy, "az-redeal", "sb-tactical"} {
		if !bots.ReadsDecklists(policy) {
			t.Fatalf("precondition: %q does not read decklists", policy)
		}
		for _, optIn := range []bool{false, true} {
			r, err := New(testOptions(t))
			if err != nil {
				t.Fatal(err)
			}
			err = r.AddTable(table(policy, []int{0}, optIn))
			if err == nil || !strings.Contains(err.Error(), "it cannot seat at a table with a human") {
				t.Errorf("%s (opt-in %v) at a human table: error %v, want the decklist refusal", policy, optIn, err)
			}
			r.Close()
		}
	}
	if bots.ReadsDecklists(azc4.Policy) {
		t.Fatal("az-c4 reads decklists")
	}
	for _, humans := range [][]int{{0}, nil} {
		r, err := New(testOptions(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := r.AddTable(table(azc4.Policy, humans, false)); err != nil {
			t.Errorf("az-c4 (humans %v) without the opt-in: %v", humans, err)
		}
		r.Close()
	}
}

// The host does not trust the declaration: an az-c4 table whose bot seat is
// an open-list search seat (here az-c3 without a belief, swapped in through
// Options.Seats) crashes at match start, before the seat is asked anything.
func TestClosedListGuardRefusesAnOpenSeatAtAHumanTable(t *testing.T) {
	enableAZC3(t)
	enableAZC4(t)
	opts := testOptions(t)
	asked := false
	opts.Seats = func(names []string, seed uint64) []seat.Seat {
		open, err := bots.New(azc3.Policy, bots.Options{Seed: seed ^ 1, AutoPayMana: true})
		if err != nil {
			panic(err)
		}
		if cl, ok := open.(searchseat.ClosedListSeat); !ok || cl.SearchBelief() != nil {
			panic("precondition: az-c3 without a belief is not an open-list seat")
		}
		return []seat.Seat{&privacySpy{inner: open, check: func(string, *decision.Decision, *bots.Env, *botpolicy.Board, *view.View) { asked = true }}, seat.NewBot(seed ^ 2)}
	}
	r, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.AddTable(TableConfig{ID: "t1", Seats: 2, Decks: []string{"a", "b"}, Seed: 1, Spectator: view.Public,
		Humans: []int{1}, BotPolicy: azc4.Policy, AutoMana: true, BotAutoPayMana: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.Start("t1"); err != nil {
		t.Fatal(err)
	}
	r.Wait("t1")
	ms, err := r.Matches("t1")
	if err != nil || len(ms) != 1 {
		t.Fatalf("matches %v, %v", ms, err)
	}
	r.mu.RLock()
	tbl := r.tables["t1"]
	r.mu.RUnlock()
	tbl.mu.RLock()
	m := tbl.history[len(tbl.history)-1]
	tbl.mu.RUnlock()
	m.mu.RLock()
	reason := m.reason
	m.mu.RUnlock()
	if ms[0].State != protocol.MatchCrashed || !strings.Contains(reason, "opponents' real decklists") {
		t.Fatalf("state %s reason %q: want a crash naming the open-list seat", ms[0].State, reason)
	}
	if asked {
		t.Fatal("the open-list seat was asked a decision before the guard refused it")
	}
}

// envData's second line: at a table with a human, an open Setup is cut to
// the actor's own list and gets no root; a closed Setup, and any Setup at a
// bot-only table, pass unchanged.
func TestClosedSetupCutsAnOpenSetupAtAHumanTable(t *testing.T) {
	b := enableAZC4(t)
	_, decks := testutil.SampleDecks(t, 2)
	open := searchprobe.PublicGame{Names: []string{"x", "y"}, Decks: decks}
	human := TableConfig{Humans: []int{1}}
	got, ok := closedSetup(human, open, 0)
	if ok || got.Decks[1] != nil || len(got.Decks[0]) != len(decks[0]) {
		t.Fatalf("open setup at a human table: ok %v, decks %d/%d", ok, len(got.Decks[0]), len(got.Decks[1]))
	}
	if open.Decks[1] == nil {
		t.Fatal("closedSetup wrote through to the caller's decks")
	}
	closed := open.Closed(0, b)
	if got, ok := closedSetup(human, closed, 0); !ok || got.Belief != b {
		t.Fatal("a closed setup was refused")
	}
	if got, ok := closedSetup(TableConfig{}, open, 0); !ok || got.Decks[1] == nil {
		t.Fatal("a bot-only table's setup was changed")
	}
}

// Two bot-only az-c4 tables from one seed (without the decklist opt-in, which
// az-c4 does not need) play the same intents byte for byte, and each replays
// to its recorded head. They are bot-only because a human answered by the
// timeout caretaker is not reproducible run to run.
func TestHostedAZC4IsDeterministic(t *testing.T) {
	enableAZC4(t)
	const seed = uint64(20261014)
	run := func(tag string) (protocol.MatchInfo, *events.Log) {
		opts := testOptions(t)
		opts.MaxIntents = detSearchIntentsCap
		opts.BotUnboundedDecisions = true
		opts.BotSearchParallelism = 4
		r, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		cfg := TableConfig{ID: "t1", Name: "azc4" + tag, Seats: 2, Decks: []string{"a", "b"},
			Seed: seed, Spectator: view.Public, BotPolicy: azc4.Policy, BotAutoPayMana: true, AutoMana: true}
		if err := r.AddTable(cfg); err != nil {
			t.Fatalf("AddTable: %v", err)
		}
		if err := r.Start("t1"); err != nil {
			t.Fatalf("Start: %v", err)
		}
		r.Wait("t1")
		r.mu.RLock()
		tbl := r.tables["t1"]
		r.mu.RUnlock()
		tbl.mu.RLock()
		m := tbl.history[0]
		tbl.mu.RUnlock()
		m.mu.RLock()
		info, log, rulesCfg, rootRefusals := m.info(), m.e.L.Clone(), m.cfg, m.rootRefusals
		m.mu.RUnlock()
		t.Logf("%s: %s turn %d intents %d, root refusals %d", tag, info.State, info.Turns, len(log.Intents), rootRefusals)
		replayed, err := replay.Replay(log, rulesCfg)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if got := replayed.L.Head(); got != info.Head {
			t.Fatalf("replay head %s, want %s", got, info.Head)
		}
		return info, log
	}
	aInfo, aLog := run("a")
	bInfo, bLog := run("b")
	if n := len(aLog.Intents); n != detSearchIntentsCap {
		t.Fatalf("run a logged %d intents, want the cap %d", n, detSearchIntentsCap)
	}
	if !reflect.DeepEqual(aLog.Intents, bLog.Intents) || aInfo.Head != bInfo.Head {
		t.Fatalf("two az-c4 runs differ (intents %d/%d, heads %s/%s)", len(aLog.Intents), len(bLog.Intents), aInfo.Head, bInfo.Head)
	}
}
