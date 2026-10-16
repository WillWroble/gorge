package host

// az-c3 (docs/design/2026-10-09-az-c3-hosted.md) through the host:
// the two gates every hosted search entry owes (spec 2026-09-28-hosted-bot-
// packages §5.3 and §8), run on the real adapter with a synthetic network.
//
//   - TestHostedEnvSeatsIgnoreTheRealHiddenCardsAZC3: the hidden-card leak row.
//     The opponent's hidden cards are swapped in the live engine at a decision
//     the seat searches; the seat's answer at that decision must not move.
//   - TestHostedAZC3IsDeterministic: two tables from one seed play the same
//     150 intents, byte for byte, and each replays to its recorded head.
//
// The policy registers only on Enable, and the registry is process-wide, so
// the first test here enables it once and every other test in this binary
// that walks bots.Entries skips it by name (TestEveryHostedPolicyIsDeterministic
// does; this file's determinism test is its row). These tests run before the
// package's parallel set resumes: the Watch hook azLeakRow owns is a global.

import (
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/gonet/gonettest"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/replay"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/server/protocol"
)

var azc3Once sync.Once

// enableAZC3 registers az-c3 over a synthetic network, once per test binary.
func enableAZC3(t *testing.T) {
	t.Helper()
	azc3Once.Do(func() {
		dir, err := os.MkdirTemp("", "azc3-host-")
		if err != nil {
			t.Fatal(err)
		}
		// The network is read whole at Enable; the file is not needed after.
		defer os.RemoveAll(dir)
		path, err := gonettest.Write(dir, "net.dzgw")
		if err != nil {
			t.Fatal(err)
		}
		if err := azc3.Enable(path); err != nil {
			t.Fatal(err)
		}
	})
	if _, ok := bots.Lookup(azc3.Policy); !ok {
		t.Fatal("az-c3 is not registered")
	}
}

func TestHostedEnvSeatsIgnoreTheRealHiddenCardsAZC3(t *testing.T) {
	enableAZC3(t)
	azLeakRow(t, azc3.Policy, func(actor uint64) seat.Seat {
		s, err := bots.New(azc3.Policy, bots.Options{Seed: actor, AutoPayMana: true})
		if err != nil {
			t.Fatalf("az-c3: %v", err)
		}
		return s
	})
}

func TestHostedAZC3IsDeterministic(t *testing.T) {
	enableAZC3(t)
	const seed = uint64(20261009)
	run := func(tag string) (protocol.MatchInfo, *events.Log) {
		opts := testOptions(t)
		opts.MaxIntents = detSearchIntentsCap
		opts.BotDeps = sampleBotDeps(t)
		// A wall-clock budget would make a deadline bail-out answer from the
		// fallback in one run and not the other: load, not seed.
		opts.BotUnboundedDecisions = true
		opts.BotSearchParallelism = 4
		r, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		cfg := TableConfig{ID: "t1", Name: "azc3" + tag, Seats: 2, Decks: []string{"a", "b"},
			Seed: seed, Spectator: view.Public, BotPolicy: azc3.Policy, BotAutoPayMana: true, AutoMana: true,
			// az-c3 reads every seat's real decklist (Info.Env); these tables are bot-only and opt in.
			AllowDecklistBots: true}
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
		if len(tbl.history) != 1 {
			t.Fatalf("table kept %d matches, want 1", len(tbl.history))
		}
		m := tbl.history[0]
		tbl.mu.RUnlock()
		m.mu.RLock()
		info, log, rulesCfg := m.info(), m.e.L.Clone(), m.cfg
		m.mu.RUnlock()
		m.mu.RLock()
		reason := m.reason
		m.mu.RUnlock()
		t.Logf("%s: %s turn %d intents %d events %d result %q reason %q", tag, info.State, info.Turns, len(log.Intents), len(log.Events), info.Result, reason)
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
	// Precondition: the run reached the cap, so the equality below compares two
	// real games and not two empty logs.
	if n := len(aLog.Intents); n != detSearchIntentsCap {
		t.Fatalf("run a logged %d intents, want the cap %d", n, detSearchIntentsCap)
	}
	if !reflect.DeepEqual(aLog.Events, bLog.Events) || !reflect.DeepEqual(aLog.Intents, bLog.Intents) {
		t.Fatalf("two az-c3 runs differ (events %d/%d, intents %d/%d, heads %s/%s)",
			len(aLog.Events), len(bLog.Events), len(aLog.Intents), len(bLog.Intents), aInfo.Head, bInfo.Head)
	}
	if aInfo.Head != bInfo.Head {
		t.Fatalf("two az-c3 runs differ in head: %s vs %s", aInfo.Head, bInfo.Head)
	}
}

// A table that names az-c3 without payment plans is refused when it is
// registered. Before the check, the factory's refusal surfaced as a panic at
// the match's first seat and crashed the match.
func TestHostRefusesAZC3WithoutAutoPay(t *testing.T) {
	enableAZC3(t)
	for _, c := range []struct {
		name             string
		autoMana, botPay bool
	}{{"neither", false, false}, {"auto_mana only", true, false}, {"bot_auto_pay_mana only", false, true}} {
		r, err := New(testOptions(t))
		if err != nil {
			t.Fatal(err)
		}
		cfg := TableConfig{ID: "t1", Name: "refused", Seats: 2, Decks: []string{"a", "b"}, Seed: 1,
			Spectator: view.Public, BotPolicy: azc3.Policy, AutoMana: c.autoMana, BotAutoPayMana: c.botPay, AllowDecklistBots: true}
		if err := r.AddTable(cfg); err == nil {
			t.Errorf("%s: AddTable accepted an az-c3 table without automatic mana payment", c.name)
		}
		r.Close()
	}
}
