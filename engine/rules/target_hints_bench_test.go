package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// hintBenchEngine plays a repo-deck game with the package test bot and returns
// a clone paused at the priority decision with the most hinted options (ties:
// the most options).
func hintBenchEngine(b *testing.B) *Engine {
	b.Helper()
	reg := testutil.CorpusRegistry(b)
	all := testutil.LegacyDeckNames()
	var best *Engine
	bestScore := -1
	for g := 0; g+1 < len(all) && g < 12; g += 2 {
		decks := [][]*cards.Card{testutil.RepoDeck(b, reg, all[g]), testutil.RepoDeck(b, reg, all[g+1])}
		e := New(Config{Seed: uint64(9300 + g), Names: []string{all[g], all[g+1]}, Decks: decks, Tokens: reg.Tokens, NameUniverse: reg.Universe()})
		e.Advance()
		bot := newTestBot(uint64(g) + 5)
		for n := 0; !e.G.Over && e.Pending() != nil && n < 3000; n++ {
			if d := e.Pending(); d.Kind == decision.KPriority {
				c := e.Clone()
				c.EnsureTargetHints()
				k := 0
				for _, o := range c.Pending().Options {
					if o.TargetHints.Present() {
						k++
					}
				}
				if score := k*1000 + len(d.Options); k > 0 && score > bestScore {
					best, bestScore = e.Clone(), score
				}
			}
			if err := e.Submit(bot.answer(e, e.Pending())); err != nil {
				b.Fatal(err)
			}
		}
	}
	if best == nil {
		b.Fatal("no position with a hinted option")
	}
	return best
}

// BenchmarkPriorityOffer is the offer walk the hints are built beside; it is
// unchanged by the hints (they are opt-in, built after the walk).
func BenchmarkPriorityOffer(b *testing.B) {
	e := hintBenchEngine(b)
	p := e.Pending().Player
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.legalActions(p)
	}
}

// BenchmarkEnsureTargetHints is the opt-in cost of the hints on the same
// position, rebuilt from scratch every iteration.
func BenchmarkEnsureTargetHints(b *testing.B) {
	e := hintBenchEngine(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.targetHintsSeq = 0
		e.EnsureTargetHints()
	}
}

// BenchmarkEnsureTargetHintsNoTarget is the same call on a priority decision
// whose options have no hostile target step: it must be a scan.
func BenchmarkEnsureTargetHintsNoTarget(b *testing.B) {
	reg := testutil.CorpusRegistry(b)
	all := testutil.LegacyDeckNames()
	e := New(Config{Seed: 1, Names: []string{all[0], all[1]},
		Decks:  [][]*cards.Card{testutil.RepoDeck(b, reg, all[0]), testutil.RepoDeck(b, reg, all[1])},
		Tokens: reg.Tokens, NameUniverse: reg.Universe()})
	e.Advance()
	for e.Pending() != nil && e.Pending().Kind != decision.KPriority {
		if err := e.Submit(newTestBot(1).answer(e, e.Pending())); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.targetHintsSeq = 0
		e.EnsureTargetHints()
	}
}
