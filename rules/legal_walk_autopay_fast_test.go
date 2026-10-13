package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestAutoPayFastAcceptMatchesPlanner is the fast path's agreement proof on a
// board it must accept: Spectral Sailor's "{3}{U}: Draw a card" over four
// untapped Islands and an empty pool. autoPayFastVerify is turned on, so the
// fast path also runs the planner and panics on any disagreement; the accept
// counter proves the fast path (not the planner) produced the offer.
//
// It is deliberately NOT parallel: it toggles the package verify flag, and a
// sequential test never overlaps a parallel one.
func TestAutoPayFastAcceptMatchesPlanner(t *testing.T) {
	old := autoPayFastVerify
	autoPayFastVerify = true
	defer func() { autoPayFastVerify = old }()

	e, _, _ := newFixtureDeck(t, 9613, ppFiller)
	e.paidManaCensus = true
	sailor := onBoardCard(t, e, 0, corpusCard(t, "Spectral Sailor"))
	for i := 0; i < 4; i++ {
		onBoard(t, e, 0, lrIsland)
	}
	before := autoPayFastAccepts.Load()
	d := ppPriority(t, e)
	if _, ok := autopayAbilityOffer(d, sailor); !ok {
		t.Fatalf("Spectral Sailor's draw not offered with four untapped Islands: %+v", d.Options)
	}
	if delta := autoPayFastAccepts.Load() - before; delta == 0 {
		t.Fatal("fast path did not accept the offered ability (dead path or a planner-only accept)")
	}
}

// TestAutoPayFastDeclinesWhenSourcesCannotPay: three Islands cannot pay
// {3}{U}, so the fast path must decline (the planner then declines too, and no
// option is offered).
func TestAutoPayFastDeclinesWhenSourcesCannotPay(t *testing.T) {
	t.Parallel()
	e, _, _ := newFixtureDeck(t, 9614, ppFiller)
	e.paidManaCensus = true
	sailor := onBoardCard(t, e, 0, corpusCard(t, "Spectral Sailor"))
	for i := 0; i < 3; i++ {
		onBoard(t, e, 0, lrIsland)
	}
	before := autoPayFastAccepts.Load()
	d := ppPriority(t, e)
	if _, ok := autopayAbilityOffer(d, sailor); ok {
		t.Fatalf("offered with three Islands: %+v", d.Options)
	}
	if delta := autoPayFastAccepts.Load() - before; delta != 0 {
		t.Fatal("fast path accepted an unpayable ability")
	}
}

// TestAutoPayFastAgreesWithPlannerInGames plays legacy-deck bot games with
// PaidManaCensus on and the verify flag set, so every fast-path accept across
// whole games is checked against the planner (a disagreement panics). It also
// requires the fast path to have accepted at least once, so a path that
// silently never fires fails the build.
//
// Sequential (it toggles the package verify flag); see the note above.
func TestAutoPayFastAgreesWithPlannerInGames(t *testing.T) {
	old := autoPayFastVerify
	autoPayFastVerify = true
	defer func() { autoPayFastVerify = old }()

	reg := testutil.CorpusRegistry(t)
	names := testutil.LegacyDeckNames()
	before := autoPayFastAccepts.Load()
	for i := 0; i < 2; i++ {
		a := names[i%len(names)]
		b := names[(i+5)%len(names)]
		cfg := Config{
			Seed:           93000 + uint64(i),
			Names:          []string{a, b},
			Decks:          [][]*cards.Card{testutil.RepoDeck(t, reg, a), testutil.RepoDeck(t, reg, b)},
			Tokens:         reg.Tokens,
			NameUniverse:   reg.Universe(),
			PaidManaCensus: true,
		}
		e := New(cfg)
		bot := newTestBot(uint64(930 + i))
		e.Advance()
		for n := 0; !e.G.Over && e.Pending() != nil && n < 4000; n++ {
			d := e.Pending()
			if err := e.Submit(bot.answer(e, d)); err != nil {
				t.Fatalf("game %d intent %d: %v", i, n, err)
			}
		}
	}
	delta := autoPayFastAccepts.Load() - before
	t.Logf("fast path accepts across %d verify games: %d", 2, delta)
	if delta == 0 {
		t.Fatal("fast path never accepted across the verify games (dead path)")
	}
}
