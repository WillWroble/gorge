package botpolicy

import (
	"slices"
	"testing"

	"github.com/adams-shaun/gorge/engine/deck"
)

func manifestWithLands(lands, total int) *deck.Manifest {
	return &deck.Manifest{Main: []deck.ManifestRow{
		{Name: "Plains", Count: lands, Land: true},
		{Name: "Spell", Count: total - lands},
	}}
}

// TestConstructedLandWindow pins the deck-derived window: hi = ceil(7*share)+2
// clamped to [4, 5], 2..5 without a manifest.
func TestConstructedLandWindow(t *testing.T) {
	for _, c := range []struct {
		lands, total, hi int
		nilDeck          bool
	}{
		{24, 60, 5, false}, {17, 40, 5, false}, {20, 60, 5, false},
		{16, 60, 4, false}, {12, 60, 4, false}, {36, 60, 5, false},
		{0, 0, 5, true},
	} {
		b := Board{}
		if !c.nilDeck {
			b.OwnDeck = manifestWithLands(c.lands, c.total)
		}
		lo, hi := b.landWindow()
		if lo != 2 || hi != c.hi {
			t.Errorf("%d/%d lands: window %d..%d, want 2..%d", c.lands, c.total, lo, hi, c.hi)
		}
	}
}

func constructedKeeps(b Board, bottom int) bool {
	d := keepMullDecision(bottom, true)
	r := rng(5)
	in := Decide(b, &d, r)
	if r.Uint64() != rng(5).Uint64() {
		panic("constructed mulligan consumed the rng")
	}
	return d.Options[in.Choices[0]].Kind == "keep"
}

// TestConstructedKeep pins the keep table: land window, an early play, and a
// producing land.
func TestConstructedKeep(t *testing.T) {
	// Land count with castable 2-drops (the sevenWith hand).
	for lands := 0; lands <= 7; lands++ {
		b := handBoard(MulliganConstructed, sevenWith(lands)...)
		b.OwnDeck = manifestWithLands(24, 60)
		want := lands >= 2 && lands <= 5
		if got := constructedKeeps(b, 0); got != want {
			t.Errorf("%d lands: keep = %v, want %v", lands, got, want)
		}
	}
	// A land-light deck mulligans five lands, a normal one keeps them.
	b := handBoard(MulliganConstructed, sevenWith(5)...)
	b.OwnDeck = manifestWithLands(16, 60)
	if constructedKeeps(b, 0) {
		t.Error("5 lands in a 16-land deck: want mulligan")
	}
	b.OwnDeck = manifestWithLands(24, 60)
	if !constructedKeeps(b, 0) {
		t.Error("5 lands in a 24-land deck: want keep")
	}
	// No early play: two Plains and five 4-drops, or white lands and black spells.
	hand := append([]Card{plainsCard(), plainsCard()}, slices.Repeat([]Card{spellCard(4, "2 W W")}, 5)...)
	if constructedKeeps(handBoard(MulliganConstructed, hand...), 0) {
		t.Error("two lands and only 4-drops: want mulligan")
	}
	hand = append([]Card{plainsCard(), plainsCard(), plainsCard()}, slices.Repeat([]Card{spellCard(2, "B B")}, 4)...)
	if constructedKeeps(handBoard(MulliganConstructed, hand...), 0) {
		t.Error("white lands and black spells: want mulligan")
	}
	// One castable spell is enough; a colourless spell with any land counts.
	hand = append([]Card{plainsCard(), swampCard(), plainsCard()}, slices.Repeat([]Card{spellCard(5, "3 W W")}, 3)...)
	hand = append(hand, spellCard(2, "1 B"))
	if !constructedKeeps(handBoard(MulliganConstructed, hand...), 0) {
		t.Error("three lands, a castable 2-drop: want keep")
	}
	hand = append([]Card{plainsCard(), plainsCard()}, slices.Repeat([]Card{spellCard(2, "2")}, 5)...)
	if !constructedKeeps(handBoard(MulliganConstructed, hand...), 0) {
		t.Error("two lands and colourless 2-drops: want keep")
	}
	// An Evolving Wilds (no recorded production) covers any colour.
	hand = append([]Card{wildsCard(), wildsCard()}, slices.Repeat([]Card{spellCard(2, "B B")}, 5)...)
	if !constructedKeeps(handBoard(MulliganConstructed, hand...), 0) {
		t.Error("two Wilds and BB 2-drops: want keep")
	}
	// After one mulligan the window still applies; after two it loosens.
	b = handBoard(MulliganConstructed, sevenWith(1)...)
	if constructedKeeps(b, 1) {
		t.Error("1 land after one mulligan: want mulligan")
	}
	if !constructedKeeps(b, 2) {
		t.Error("1 land after two mulligans: want keep (seven hold a land and a spell)")
	}
	if constructedKeeps(handBoard(MulliganConstructed, sevenWith(0)...), 2) {
		t.Error("no land after two mulligans: want mulligan")
	}
	if !constructedKeeps(handBoard(MulliganConstructed, sevenWith(0)...), 3) {
		t.Error("three mulligans: always keep")
	}
	// A keep-only offer keeps, a census mismatch keeps.
	b = handBoard(MulliganConstructed, sevenWith(0)...)
	d := keepMullDecision(2, false)
	if in := Decide(b, &d, rng(1)); in.Choices[0] != 0 {
		t.Errorf("keep-only offer = %+v", in)
	}
	b.HandSize = 8
	if !constructedKeeps(b, 0) {
		t.Error("census mismatch: want keep")
	}
}

// TestConstructedBottoming pins that the rule bottoms exactly as
// MulliganLands does, and that MulliganAnswer matches Decide.
func TestConstructedBottoming(t *testing.T) {
	hand := sevenWith(5)
	d := bottomDecision(7, 2)
	got := Decide(handBoard(MulliganConstructed, hand...), &d, rng(1)).Choices
	want := Decide(handBoard(MulliganLands, hand...), &d, rng(1)).Choices
	if !slices.Equal(got, want) || len(got) != 2 {
		t.Errorf("constructed bottoming = %v, want lands' %v", got, want)
	}
	b := handBoard(MulliganConstructed, sevenWith(1)...)
	kd := keepMullDecision(0, true)
	c, ok := MulliganAnswer(b, MulliganConstructed, &kd)
	if !ok || !slices.Equal(c, []int{1}) {
		t.Errorf("MulliganAnswer 1-land hand = %v %v, want mulligan", c, ok)
	}
	if _, ok := MulliganAnswer(b, MulliganCoin, &kd); ok {
		t.Error("MulliganAnswer answered under the coin rule")
	}
}
