// Package build turns a drafted pool into an engine-loadable deck: the
// spec §4.5 half of the draft stack. It is pure — a deck is validated
// against the pool it came from, lands are split by coloured-pip share and
// the result is written out as a deck.File. It imports only the limited
// vocabulary and the deck-list format.
package build

import (
	"fmt"
	"sort"

	"github.com/adams-shaun/gorge/deck"
	"github.com/adams-shaun/gorge/limited"
)

// BasicNames names the five basics in the fixed W U B R G order every
// Basics array uses.
var BasicNames = [5]string{"Plains", "Island", "Swamp", "Mountain", "Forest"}

// Deck is one seat's built deck: the CardRef IDs it maindecks from its own
// pool plus the basic lands it adds free of charge (MTR 7.1 — basics are
// unlimited, and there is no other copy limit in limited play).
type Deck struct {
	// Main is the deck's non-basic cards, as IDs of the pool's CardRefs.
	Main []string
	// Basics counts each basic land by colour: W, U, B, R, G.
	Basics [5]int
}

// Total is the deck's card count: maindeck cards plus basics.
func (d Deck) Total() int {
	n := len(d.Main)
	for _, c := range d.Basics {
		n += c
	}
	return n
}

// Validate checks a deck against the pool it was built from: at least 40
// cards, every maindeck ID in the pool, no duplicate IDs. There is no
// maximum and no copy limit (MTR 7.1); basics are free and unlimited.
func Validate(pool []limited.CardRef, d Deck) error {
	inPool := make(map[string]struct{}, len(pool))
	for _, c := range pool {
		inPool[c.ID] = struct{}{}
	}
	if total := d.Total(); total < 40 {
		return fmt.Errorf("build: deck has %d cards, want at least 40", total)
	}
	seen := make(map[string]struct{}, len(d.Main))
	for _, id := range d.Main {
		if _, ok := inPool[id]; !ok {
			return fmt.Errorf("build: card %q is not in the pool", id)
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("build: card %q is maindecked twice", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

// AutoLands fills a maindeck up to 40 cards with basics, split by the
// coloured-pip share of the non-land main cards using largest remainder.
// colourOf reports, for one maindeck card ID, how many pips of each colour
// (W U B R G) its mana costs carry; a nil colourOf counts no pips. A deck
// with no coloured pips — all colourless spells, or no land slots to fill —
// splits its share over nothing and gets zero basics back.
func AutoLands(main []string, colourOf func(id string) [5]int) [5]int {
	var out [5]int
	lands := 40 - len(main)
	if lands <= 0 {
		return out
	}
	if colourOf == nil {
		colourOf = func(string) [5]int { return [5]int{} }
	}
	var pips [5]int
	total := 0
	for _, id := range main {
		for c, p := range colourOf(id) {
			pips[c] += p
			total += p
		}
	}
	if total == 0 {
		return out
	}
	// exact[i] = lands * pips[i] / total, kept in integer parts: floor plus
	// the remainder (lands*pips[i] mod total) largest remainders are handed
	// the leftover lands, ties to the lower colour index.
	var rem [5]int
	left := lands
	for i := range out {
		exact := lands * pips[i]
		out[i] = exact / total
		rem[i] = exact % total
		left -= out[i]
	}
	idx := []int{0, 1, 2, 3, 4}
	sort.SliceStable(idx, func(a, b int) bool {
		if rem[idx[a]] != rem[idx[b]] {
			return rem[idx[a]] > rem[idx[b]]
		}
		return idx[a] < idx[b]
	})
	for _, i := range idx {
		if left == 0 {
			break
		}
		out[i]++
		left--
	}
	return out
}

// ToDeckFile writes the deck out as an engine-loadable deck file, with the
// basics as ordinary entries. Card names come from the pool by ID; an ID
// the pool does not carry yields an empty name, so run Validate first.
// The file's Format and Archetype stay empty: they are authoring metadata
// the caller may fill in.
func ToDeckFile(pool []limited.CardRef, d Deck, name string) deck.File {
	nameOf := make(map[string]string, len(pool))
	for _, c := range pool {
		nameOf[c.ID] = c.Name
	}
	f := deck.File{Name: name}
	for _, id := range d.Main {
		f.Cards = append(f.Cards, deck.Entry{Name: nameOf[id], Count: 1})
	}
	for i, n := range d.Basics {
		if n > 0 {
			f.Cards = append(f.Cards, deck.Entry{Name: BasicNames[i], Count: n})
		}
	}
	return f
}
