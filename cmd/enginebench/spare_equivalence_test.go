package main

import (
	"reflect"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/testkit/bench"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

func TestEnginebenchPooledRowsMatchFresh(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	a, err := testutil.LoadRepoDeck(reg, "mono-black-aggro")
	if err != nil {
		t.Fatal(err)
	}
	b, err := testutil.LoadRepoDeck(reg, "mono-red-prowess")
	if err != nil {
		t.Fatal(err)
	}
	cfg := rules.Config{Seed: 77123, Names: []string{"a", "b"}, Decks: [][]*cards.Card{a, b}, Tokens: reg.Tokens}
	var randomFresh, randomPooled randomStats
	playRandomWithPool(cfg, &randomFresh, nil)
	var randomPool bench.SparePool
	playRandomWithPool(cfg, &randomPooled, &randomPool)
	if !reflect.DeepEqual(randomFresh, randomPooled) {
		t.Fatalf("random row changed with spare recycling:\nfresh=%+v\npooled=%+v", randomFresh, randomPooled)
	}
	var botFresh, botPooled botStats
	if err := playBotWithPool(cfg, false, &botFresh, nil); err != nil {
		t.Fatal(err)
	}
	var botPool bench.SparePool
	if err := playBotWithPool(cfg, false, &botPooled, &botPool); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(botFresh, botPooled) {
		t.Fatalf("bot row changed with spare recycling:\nfresh=%+v\npooled=%+v", botFresh, botPooled)
	}
	if randomFresh.Games != 1 || botFresh.Games != 1 {
		t.Fatal("precondition: row did not complete exactly one game")
	}
}

// TestEnginebenchAutopayRandomRow pins the random row's -autopay answer: it
// recycles spares as faithfully as the manual row, and every draw it makes is
// an answer the engine accepts (no rejection, no heuristic fallback).
func TestEnginebenchAutopayRandomRow(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	a, err := testutil.LoadRepoDeck(reg, "mono-black-aggro")
	if err != nil {
		t.Fatal(err)
	}
	b, err := testutil.LoadRepoDeck(reg, "mono-red-prowess")
	if err != nil {
		t.Fatal(err)
	}
	var fresh, pooled randomStats
	var pool bench.SparePool
	for g := uint64(0); g < 4; g++ {
		cfg := rules.Config{Seed: 77123 + g, Names: []string{"a", "b"}, Decks: [][]*cards.Card{a, b}, Tokens: reg.Tokens}
		playRandomGame(cfg, &fresh, nil, true)
		playRandomGame(cfg, &pooled, &pool, true)
	}
	if !reflect.DeepEqual(fresh, pooled) {
		t.Fatalf("autopay random row changed with spare recycling:\nfresh=%+v\npooled=%+v", fresh, pooled)
	}
	if fresh.Games != 4 || fresh.Stalls != 0 || fresh.Rejected != 0 || fresh.Fallbacks != 0 {
		t.Fatalf("autopay random row: want 4 clean games, got %+v", fresh)
	}
}
