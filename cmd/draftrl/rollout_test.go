package main

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/limited/pickbot"
)

// TestRolloutSmokeBot is DR-7's smoke: 1 pod, the bot pilot, 2 games per
// pair. Every record file is written, the round robin is complete, and a
// second run from the same seed at a different worker count reproduces
// every record byte for byte.
func TestRolloutSmokeBot(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	outA, outB := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	args := func(out, workers string) []string {
		return []string{"-corpus", dir, "-pods", "1", "-k", "2", "-pilot", "bot", "-seed", "1000000",
			"-league", "17l,17l", "-workers", workers, "-out", out}
	}
	var log bytes.Buffer
	r, err := rollout(args(outA, "4"), &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if len(r.Seats) != 4 || len(r.Games) != 12 || len(r.Picks) != 4*3*14 {
		t.Fatalf("seats %d games %d picks %d, want 4/12/168", len(r.Seats), len(r.Games), len(r.Picks))
	}
	for _, sr := range r.Seats {
		if sr.Total.Games != 6 || len(sr.Results) != 3 {
			t.Fatalf("seat %d: %d games over %d opponents, want 6 over 3", sr.Seat, sr.Total.Games, len(sr.Results))
		}
		if sr.Pair < 0 || sr.PairName == "" || len(sr.Main) != 40 {
			t.Fatalf("seat %d: pair %d %q main %d", sr.Seat, sr.Pair, sr.PairName, len(sr.Main))
		}
	}
	for _, p := range r.Picks {
		if p.ChosenIdx < 0 || p.Chosen == "" || len(p.Features.Cands) != len(p.Pack) {
			t.Fatalf("pick %+v: chosen %d cands %d pack %d", p.Pick, p.ChosenIdx, len(p.Features.Cands), len(p.Pack))
		}
	}
	if _, err := rollout(args(outB, "1"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"picks.jsonl", "picks.bin", "seats.jsonl", "games.jsonl", "game_data.csv", "table.json", "manifest.json"} {
		a, err := os.ReadFile(filepath.Join(outA, f))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(outB, f))
		if err != nil {
			t.Fatal(err)
		}
		if len(a) == 0 || !bytes.Equal(a, b) {
			t.Fatalf("%s: not reproduced from the seed (%d vs %d bytes)", f, len(a), len(b))
		}
	}
	bin, _ := os.ReadFile(filepath.Join(outA, "picks.bin"))
	if string(bin[:4]) != "DRPB" || binary.LittleEndian.Uint32(bin[8:]) != uint32(len(globalLayout)) {
		t.Fatalf("picks.bin header %q", bin[:16])
	}
	if !strings.Contains(log.String(), "games/s") {
		t.Fatalf("no throughput line:\n%s", log.String())
	}
}

// TestRolloutExcludesGateDecks is the operator ruling's guard: no
// fdn-limited gauntlet deck can enter a rollout. Every catalog deck, under
// any name, trips the card-list-hash guard; a rollout-built deck does not.
func TestRolloutExcludesGateDecks(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	lab, err := draftlab.Open(dir, draftlab.Options{})
	if err != nil {
		t.Fatal(err)
	}
	guard, err := newGateDeckGuard(lab)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := lab.CatalogDecks("fdn-limited")
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) == 0 || len(guard) == 0 {
		t.Fatalf("catalog %d decks, guard %d hashes", len(cat), len(guard))
	}
	for _, d := range cat {
		d.Name = "p1000000-s0" // renamed: the hash, not the name, is the key
		// Reordered list: the hash must not depend on entry order.
		cards := append(d.File.Cards[:0:0], d.File.Cards...)
		rand.New(rand.NewPCG(1, 2)).Shuffle(len(cards), func(i, j int) { cards[i], cards[j] = cards[j], cards[i] })
		d.File.Cards = cards
		if err := guard.check(d); err == nil {
			t.Fatalf("gauntlet deck %s passed the guard", d.Source)
		}
	}
	packs, err := lab.OpenPod("FDN", "", 1000000, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := lab.Drafter(draftlab.AgentRef{Name: "rating-colour"})
	dr, err := lab.Draft(packs, []draftlab.NamedDrafter{d}, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := lab.Builder(draftlab.AgentRef{Name: "best23"})
	deck, err := lab.Build(dr.Pools[0], b)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.check(deck); err != nil {
		t.Fatal(err)
	}
}

// fakeNet is a stand-in network drafter for the DrafterKind hook: it takes
// the first card and reports one logit per pack card.
type fakeNet struct{ last []float32 }

func (f *fakeNet) Pick(v pickbot.PickView, _ *rand.Rand) string {
	f.last = make([]float32, len(v.Pack))
	f.last[0] = 1
	return v.Pack[0].ID
}
func (f *fakeNet) LastLogits() []float32 { return f.last }

func fakeKind(_ *draftlab.Lab, arg, _ string) (pickbot.Drafter, draftlab.AgentRef, error) {
	return &fakeNet{}, draftlab.AgentRef{Name: "fake", Version: arg}, nil
}

// TestRolloutDrafterKindHook drafts with a registered kind and checks its
// logits ride the pick records.
func TestRolloutDrafterKindHook(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	RegisterDrafterKind("fake", fakeKind)
	if _, err := parseLeague("nokind:x"); err == nil {
		t.Fatal("unregistered kind parsed")
	}
	r, err := rollout([]string{"-corpus", dir, "-pods", "1", "-k", "1", "-pilot", "bot",
		"-league", "fake:v1,17l,17l,17l", "-league-fixed", "-out", t.TempDir()}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Picks {
		if p.Seat == 0 && (p.Drafter != "fake:v1" || p.ChosenIdx != 0 || len(p.Logits) != len(p.Pack)) {
			t.Fatalf("seat 0 pick %d: drafter %s chosen %d logits %d", p.Pick, p.Drafter, p.ChosenIdx, len(p.Logits))
		}
		if p.Seat != 0 && p.Logits != nil {
			t.Fatalf("seat %d recorded logits", p.Seat)
		}
	}
	if r.Seats[0].DrafterRef != "fake@v1" {
		t.Fatalf("drafter ref %q", r.Seats[0].DrafterRef)
	}
}
