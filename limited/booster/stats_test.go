package booster_test

import (
	"math/rand/v2"
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/booster"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// wubrgLetters spells the five colours in the order the colour helper emits.
const wubrgLetters = "WUBRG"

// corpusColourOf builds a booster.ColourOf from the card corpus registry:
// each card's colour identity bitmask (cards.Face's W/U/B/R/G bits) becomes
// its colour byte slice, cached per name. A name that does not resolve is a
// test failure, not a silent colourless card.
func corpusColourOf(t *testing.T, reg *cards.Registry) booster.ColourOf {
	t.Helper()
	cache := map[string][]byte{}
	return func(name string) []byte {
		if cs, ok := cache[name]; ok {
			return cs
		}
		card, ok := reg.Lookup(name)
		if !ok {
			t.Errorf("corpusColourOf: %q does not resolve in the corpus", name)
			return nil
		}
		id := card.ColourIdentity()
		var cs []byte
		for i, bit := range []uint8{cards.ColourWhite, cards.ColourBlue, cards.ColourBlack, cards.ColourRed, cards.ColourGreen} {
			if id&bit != 0 {
				cs = append(cs, wubrgLetters[i])
			}
		}
		cache[name] = cs
		return cs
	}
}

// slotLayout locates the pack positions the stats asserts read: where the
// rare/mythic slot, the wildcard slots and the colour-balanced common slot
// sit once the slots fill in order.
type slotLayout struct {
	rarePos               int
	wildcardPositions     []int
	commonsStart, commons int
	rareMythic            setdata.RareMythic
	wildcardWeights       []setdata.Weights
}

func slotLayoutOf(t *testing.T, spec setdata.Spec) slotLayout {
	t.Helper()
	var l slotLayout
	l.rarePos = -1
	pos := 0
	for _, slot := range spec.Slots {
		switch {
		case slot.RareMythic != nil:
			if l.rarePos >= 0 {
				t.Fatal("precondition: spec has more than one rare_mythic slot")
			}
			l.rarePos = pos
			l.rareMythic = *slot.RareMythic
		case slot.Wildcard != nil:
			for n := 0; n < slot.Count; n++ {
				l.wildcardPositions = append(l.wildcardPositions, pos+n)
			}
			l.wildcardWeights = append(l.wildcardWeights, *slot.Wildcard)
		case slot.BalanceColours:
			l.commonsStart, l.commons = pos, slot.Count
		}
		pos += slot.Count
	}
	if l.rarePos < 0 || len(l.wildcardPositions) == 0 || l.commons == 0 {
		t.Fatalf("precondition: spec %s lacks a rare_mythic, wildcard or balance_colours slot", spec.Code)
	}
	return l
}

// assertMonoOnSheet is the live-precondition check behind the colour
// coverage assert: a balanced common slot can only cover colours the sheet
// actually carries mono cards of.
func assertMonoOnSheet(t *testing.T, code, sheet string, cardsOnSheet []setdata.SheetCard, colourOf booster.ColourOf) map[byte]int {
	t.Helper()
	counts := map[byte]int{}
	for _, sc := range cardsOnSheet {
		cs := colourOf(sc.Name)
		if len(cs) == 1 {
			counts[cs[0]]++
		}
	}
	for i := 0; i < len(wubrgLetters); i++ {
		c := wubrgLetters[i]
		if counts[c] == 0 {
			t.Fatalf("precondition: %s %s sheet carries no mono-%c card, so a colour-coverage assert could never pass", code, sheet, c)
		}
	}
	return counts
}

// openPacks opens n packs of one spec from independent PCG streams under one
// seed, sharing one event-wide ID allocator, and runs the structural asserts
// every pack must pass: full size, no repeated name, ID uniqueness.
func openPacks(t *testing.T, spec setdata.Spec, seed uint64, n int, colourOf booster.ColourOf, ids *booster.IDs) [][]limited.CardRef {
	t.Helper()
	out := make([][]limited.CardRef, n)
	for i := range out {
		pack, err := booster.Open(spec, rand.New(rand.NewPCG(seed, uint64(i))), ids, colourOf)
		if err != nil {
			t.Fatalf("Open pack %d: %v", i, err)
		}
		if len(pack) != spec.PackSize {
			t.Fatalf("pack %d has %d cards, want %d", i, len(pack), spec.PackSize)
		}
		seen := map[string]bool{}
		for _, c := range pack {
			if seen[c.Name] {
				t.Fatalf("pack %d repeats %q", i, c.Name)
			}
			if seen[c.ID] {
				t.Fatalf("pack %d reuses ID %q", i, c.ID)
			}
			seen[c.Name] = true
			seen[c.ID] = true
		}
		out[i] = pack
	}
	return out
}

// TestBoosterFDNStats is the spec §4.3 statistics gate over 10,000
// Foundations packs from a fixed seed: 14 cards each, no duplicate names,
// the mythic share of the rare/mythic slot inside [0.13, 0.16], the wildcard
// rarity shares within ±0.02 of their weights, and all five colours covered
// by mono commons in ≥99% of packs — plus the same seed opening an identical
// pack set.
func TestBoosterFDNStats(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	if reg.Len() == 0 {
		t.Fatal("corpus registry is empty: the stats gate measured nothing (is .cards present?)")
	}
	spec, err := setdata.Load("FDN")
	if err != nil {
		t.Fatalf("Load(FDN): %v", err)
	}
	colourOf := corpusColourOf(t, reg)
	if got := len(spec.Sheets["common"]); got != 80 {
		t.Fatalf("precondition: FDN common sheet has %d cards, want 80", got)
	}
	assertMonoOnSheet(t, "FDN", "common", spec.Sheets["common"], colourOf)
	layout := slotLayoutOf(t, spec)
	if layout.commons != 7 {
		t.Fatalf("precondition: FDN balance_colours slot draws %d cards, want 7", layout.commons)
	}

	const packs = 10000
	const seed = 0xFD00F00D
	ids := &booster.IDs{}
	all := openPacks(t, spec, seed, packs, colourOf, ids)
	if ids.Count() != packs*spec.PackSize {
		t.Errorf("allocator handed out %d IDs, want %d (one per card, event-wide)", ids.Count(), packs*spec.PackSize)
	}

	// Mythic share of the rare/mythic slot: 60 R at weight 2 vs 20 M at
	// weight 1 gives 1/7 ≈ 0.143.
	mythics := 0
	for _, pack := range all {
		if pack[layout.rarePos].Rarity == limited.Mythic {
			mythics++
		}
	}
	mythicShare := float64(mythics) / packs
	t.Logf("mythic share of the FDN rare/mythic slot: %.4f", mythicShare)
	if mythicShare < 0.13 || mythicShare > 0.16 {
		t.Errorf("mythic share of the rare/mythic slot = %.4f, want [0.13, 0.16]", mythicShare)
	}

	// Wildcard rarity shares against the slot's own weights.
	var wildRarity [4]int // C U R M, aggregated over every wildcard draw
	for _, pack := range all {
		for _, pos := range layout.wildcardPositions {
			switch pack[pos].Rarity {
			case limited.Common:
				wildRarity[0]++
			case limited.Uncommon:
				wildRarity[1]++
			case limited.Rare:
				wildRarity[2]++
			case limited.Mythic:
				wildRarity[3]++
			default:
				t.Fatalf("wildcard drew rarity %c outside C/U/R/M", pack[pos].Rarity)
			}
		}
	}
	wildDraws := float64(len(all) * len(layout.wildcardPositions))
	rmTotal := float64(layout.rareMythic.RareWeight + layout.rareMythic.MythicWeight)
	want := [4]float64{layout.wildcardWeights[0].Common, layout.wildcardWeights[0].Uncommon,
		layout.wildcardWeights[0].RareMythic * float64(layout.rareMythic.RareWeight) / rmTotal,
		layout.wildcardWeights[0].RareMythic * float64(layout.rareMythic.MythicWeight) / rmTotal}
	for i, w := range want {
		share := float64(wildRarity[i]) / wildDraws
		t.Logf("wildcard rarity %c share: %.4f (weight %.4f)", "CURM"[i], share, w)
		if share < w-0.02 || share > w+0.02 {
			t.Errorf("wildcard rarity %c share = %.4f, want %.4f ± 0.02", "CURM"[i], share, w)
		}
	}

	// Colour coverage: every pack's 7 balanced commons include a
	// mono-coloured card of each of W/U/B/R/G.
	covered := 0
	for _, pack := range all {
		mono := monoCount(colourOf, pack[layout.commonsStart:layout.commonsStart+layout.commons])
		full := true
		for _, n := range mono {
			if n == 0 {
				full = false
				break
			}
		}
		if full {
			covered++
		}
	}
	rate := float64(covered) / packs
	t.Logf("FDN packs whose 7 commons cover all five colours: %.4f", rate)
	if rate < 0.99 {
		t.Errorf("five-colour coverage of the commons = %.4f, want ≥ 0.99", rate)
	}

	// Same seed ⇒ identical output: reopen every pack and compare card for
	// card (CardRef is comparable; a fresh allocator keeps the IDs aligned).
	ids2 := &booster.IDs{}
	again := openPacks(t, spec, seed, packs, colourOf, ids2)
	for i := range all {
		for j := range all[i] {
			if all[i][j] != again[i][j] {
				t.Fatalf("pack %d card %d differs between two opens of seed %#x: %+v vs %+v", i, j, seed, all[i][j], again[i][j])
			}
		}
	}
}

// TestBoosterBLBSmoke runs the structural half of the stats gate over 1,000
// Bloomburrow packs: full packs, no duplicate names, a rare/mythic-only rare
// slot, wildcard rarities inside C/U/R/M, and five-colour commons coverage
// logged and asserted at the same ≥99% bar.
func TestBoosterBLBSmoke(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	if reg.Len() == 0 {
		t.Fatal("corpus registry is empty: the smoke gate checked nothing (is .cards present?)")
	}
	spec, err := setdata.Load("BLB")
	if err != nil {
		t.Fatalf("Load(BLB): %v", err)
	}
	colourOf := corpusColourOf(t, reg)
	assertMonoOnSheet(t, "BLB", "common", spec.Sheets["common"], colourOf)
	layout := slotLayoutOf(t, spec)

	const packs = 1000
	const seed = 0xB10B
	all := openPacks(t, spec, seed, packs, colourOf, &booster.IDs{})
	for i, pack := range all {
		if r := pack[layout.rarePos].Rarity; r != limited.Rare && r != limited.Mythic {
			t.Errorf("BLB pack %d rare slot holds %q at rarity %c", i, pack[layout.rarePos].Name, r)
		}
		for _, pos := range layout.wildcardPositions {
			switch pack[pos].Rarity {
			case limited.Common, limited.Uncommon, limited.Rare, limited.Mythic:
			default:
				t.Errorf("BLB pack %d wildcard holds %q at rarity %c", i, pack[pos].Name, pack[pos].Rarity)
			}
		}
	}
	covered := 0
	for _, pack := range all {
		mono := monoCount(colourOf, pack[layout.commonsStart:layout.commonsStart+layout.commons])
		full := true
		for _, n := range mono {
			if n == 0 {
				full = false
				break
			}
		}
		if full {
			covered++
		}
	}
	rate := float64(covered) / packs
	t.Logf("BLB packs whose commons cover all five colours: %.4f", rate)
	if rate < 0.99 {
		t.Errorf("BLB five-colour coverage of the commons = %.4f, want ≥ 0.99", rate)
	}
}
