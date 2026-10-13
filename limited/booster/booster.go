// Package booster opens seeded Play Booster packs from a limited/setdata
// Spec (draft-events spec §4.3). It fills the spec's slots in order, never
// puts the same card name twice in one pack, draws wildcard rarities by the
// spec's weights, and colour-balances the slots marked balance_colours.
//
// The package is pure: its only randomness is the *rand.Rand the caller
// seeds, its only inputs besides that are the spec and a colour lookup the
// caller supplies, and it imports nothing but the standard library and its
// limited/ siblings.
package booster

import (
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// ColourOf reports a card's colours as W/U/B/R/G bytes, in that order. A
// mono-coloured card returns exactly one byte, a colourless card an empty
// (or nil) slice, a multicoloured card one byte per colour. The lookup is a
// function so the package never has to touch the card corpus itself.
type ColourOf func(name string) []byte

// IDSource hands out the card IDs a pack's CardRefs carry. The caller
// supplies one allocator per event so IDs stay unique across every pack it
// opens.
type IDSource interface {
	Next() string
}

// IDs is the simple sequential allocator the event layer uses: zero value
// ready, first Next is "c1".
type IDs struct{ next int }

// Next returns the next sequential ID ("c1", "c2", ...).
func (ids *IDs) Next() string {
	ids.next++
	return fmt.Sprintf("c%d", ids.next)
}

// Count reports how many IDs the allocator has handed out.
func (ids *IDs) Count() int { return ids.next }

// wubrg spells the five colours in the order colourOf's bytes and every
// deterministic tie-break in this package use.
const wubrg = "WUBRG"

// Open fills one pack from spec. rng drives every draw; ids assigns the
// CardRef IDs in slot-fill order; colourOf backs the colour balance of the
// slots whose BalanceColours is set (Open refuses to run when such a slot
// exists and colourOf is nil).
//
// Every draw resamples a name already in the pack, so a pack never carries
// the same name twice. A slot that cannot be filled without duplicates
// fails Open with an error.
func Open(spec setdata.Spec, rng *rand.Rand, ids IDSource, colourOf ColourOf) ([]limited.CardRef, error) {
	if rng == nil {
		return nil, errors.New("booster: Open needs a seeded *rand.Rand")
	}
	if ids == nil {
		return nil, errors.New("booster: Open needs a non-nil ID allocator")
	}
	for i, slot := range spec.Slots {
		if slot.BalanceColours && colourOf == nil {
			return nil, fmt.Errorf("booster: slot %d balances colours but no ColourOf was supplied", i)
		}
	}
	pack := make([]limited.CardRef, 0, spec.PackSize)
	inPack := map[string]bool{}
	for i, slot := range spec.Slots {
		start := len(pack)
		for n := 0; n < slot.Count; n++ {
			var (
				card setdata.SheetCard
				err  error
			)
			switch {
			case slot.Sheet != "":
				card, err = drawSheetUnique(rng, spec.Sheets[slot.Sheet], inPack)
			case slot.RareMythic != nil:
				card, err = drawRareMythic(rng, spec.Sheets, slot.RareMythic, inPack)
			case slot.Wildcard != nil:
				card, err = drawWildcard(rng, spec, slot.Wildcard, inPack)
			default:
				err = fmt.Errorf("slot %d sets none of sheet/rare_mythic/wildcard", i)
			}
			if err != nil {
				return nil, fmt.Errorf("booster: %s slot %d: %w", spec.Code, i, err)
			}
			pack = append(pack, limited.CardRef{
				ID:     ids.Next(),
				Name:   card.Name,
				Set:    spec.Code,
				Number: card.Number,
				Rarity: card.Rarity,
			})
			inPack[card.Name] = true
		}
		if slot.BalanceColours {
			balanceColours(rng, pack[start:], spec.Sheets[slot.Sheet], colourOf, inPack)
		}
	}
	return pack, nil
}

// freeIndices returns the sheet positions whose names are not yet in the
// pack, in sheet order.
func freeIndices(sheet []setdata.SheetCard, inPack map[string]bool) []int {
	free := make([]int, 0, len(sheet))
	for i, sc := range sheet {
		if !inPack[sc.Name] {
			free = append(free, i)
		}
	}
	return free
}

// drawSheetUnique draws a uniformly random sheet card whose name is not yet
// in the pack. It fails only when every card on the sheet is already packed.
func drawSheetUnique(rng *rand.Rand, sheet []setdata.SheetCard, inPack map[string]bool) (setdata.SheetCard, error) {
	free := freeIndices(sheet, inPack)
	if len(free) == 0 {
		return setdata.SheetCard{}, errors.New("every card on the sheet is already in the pack")
	}
	return sheet[free[rng.IntN(len(free))]], nil
}

// rareMythicWeights returns the weights a rare/mythic split draws on: the
// first RareMythic slot's weights in spec order, or 1:1 when the spec has no
// such slot to inherit from.
func rareMythicWeights(spec setdata.Spec) setdata.RareMythic {
	for _, slot := range spec.Slots {
		if slot.RareMythic != nil {
			return *slot.RareMythic
		}
	}
	return setdata.RareMythic{RareWeight: 1, MythicWeight: 1}
}

// drawRareMythic picks one card across the rare and mythic sheets, each rare
// carrying RareWeight and each mythic MythicWeight (2:1 per-card weights over
// 60 R / 20 M give the published one-mythic-in-seven rare slots). The draw
// excludes names already in the pack and fails when nothing is left.
func drawRareMythic(rng *rand.Rand, sheets map[string][]setdata.SheetCard, rm *setdata.RareMythic, inPack map[string]bool) (setdata.SheetCard, error) {
	freeR := freeIndices(sheets["rare"], inPack)
	freeM := freeIndices(sheets["mythic"], inPack)
	spanR, spanM := len(freeR)*rm.RareWeight, len(freeM)*rm.MythicWeight
	if spanR+spanM == 0 {
		return setdata.SheetCard{}, errors.New("no unpacked rare or mythic card is left")
	}
	n := rng.IntN(spanR + spanM)
	if n < spanR {
		return sheets["rare"][freeR[n/rm.RareWeight]], nil
	}
	return sheets["mythic"][freeM[(n-spanR)/rm.MythicWeight]], nil
}

// drawWildcard first rolls the slot's rarity by its weights, then draws a
// card of that rarity: common and uncommon straight off their sheets, the
// rare_mythic bucket split by the spec's per-card rare/mythic weights.
func drawWildcard(rng *rand.Rand, spec setdata.Spec, w *setdata.Weights, inPack map[string]bool) (setdata.SheetCard, error) {
	roll := rng.Float64()
	switch {
	case roll < w.Common:
		return drawSheetUnique(rng, spec.Sheets["common"], inPack)
	case roll < w.Common+w.Uncommon:
		return drawSheetUnique(rng, spec.Sheets["uncommon"], inPack)
	default:
		rm := rareMythicWeights(spec)
		return drawRareMythic(rng, spec.Sheets, &rm, inPack)
	}
}

// balanceColours tops up the colour coverage of one balance_colours slot's
// freshly drawn cards (spec §4.3): while some W/U/B/R/G colour has no
// mono-coloured card in the slot, it replaces one card with an unpacked mono
// card of the missing colour, up to five tries. The replaced card is a
// colourless or multicoloured one when the slot has any (swapping a mono
// card for a mono card of another colour can never reach five covered
// colours, and dropping the victim costs no coverage, so each try strictly
// gains a colour); when the whole slot is mono, it is a card of the
// most-represented colour as the spec spells it — with every colour covered
// but one and seven mono cards in play, that colour holds at least two, so
// the trade empties nothing. Every choice is deterministic (missing colour,
// most-represented colour and replaced position all break ties in
// WUBRG/slot order); only the replacement card is random. A colour the
// sheet has no unpacked mono card of stops the balancing quietly — the pack
// stays as drawn.
func balanceColours(rng *rand.Rand, slot []limited.CardRef, sheet []setdata.SheetCard, colourOf ColourOf, inPack map[string]bool) {
	for tries := 0; tries < 5; tries++ {
		mono := [5]int{}
		for i := range slot {
			if c := monoColour(colourOf(slot[i].Name)); c >= 0 {
				mono[c]++
			}
		}
		missing, best := -1, 0
		for c := 0; c < 5; c++ {
			if mono[c] == 0 && missing < 0 {
				missing = c
			}
			if mono[c] > mono[best] {
				best = c
			}
		}
		if missing < 0 {
			return
		}
		// Candidates: unpacked mono cards of the missing colour, in sheet order.
		var cand []int
		for i, sc := range sheet {
			if !inPack[sc.Name] && monoColour(colourOf(sc.Name)) == missing {
				cand = append(cand, i)
			}
		}
		if len(cand) == 0 {
			return
		}
		// Victim: the first non-mono card — losing it costs no colour
		// coverage and each try then strictly gains a colour. When the whole
		// slot is mono, the spec's most-represented colour, which with six
		// mono cards left after one trade holds at least two copies.
		pos := -1
		for i := range slot {
			if monoColour(colourOf(slot[i].Name)) < 0 {
				pos = i
				break
			}
		}
		if pos < 0 {
			if mono[best] < 2 {
				return // nothing to trade away without emptying a colour
			}
			for i := range slot {
				if monoColour(colourOf(slot[i].Name)) == best {
					pos = i
					break
				}
			}
		}
		if pos < 0 {
			return
		}
		sc := sheet[cand[rng.IntN(len(cand))]]
		delete(inPack, slot[pos].Name)
		slot[pos] = limited.CardRef{ID: slot[pos].ID, Name: sc.Name, Set: slot[pos].Set, Number: sc.Number, Rarity: sc.Rarity}
		inPack[sc.Name] = true
	}
}

// monoColour returns the WUBRG index of cs's single colour, or -1 when cs is
// not exactly one of the five colours.
func monoColour(cs []byte) int {
	if len(cs) != 1 {
		return -1
	}
	for i := 0; i < len(wubrg); i++ {
		if wubrg[i] == cs[0] {
			return i
		}
	}
	return -1
}
