package searchprobe

import (
	"errors"
	"math/rand/v2"
	"sort"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/state"
)

// Belief is the closed-decklist redeal's model of what an opponent might be
// playing: a corpus of deck lists the seat could plausibly face (for the
// FDN self-play loop, the TRAIN split of the deck pool). A closed world
// draws one list from it per deal, conditioned only on what the seat saw,
// and deals the opponent's unknown hand and library from that list
// (Redealer, PublicGame.Belief) -- so a world never reads the opponent's
// real list or its real hidden cards.
//
// The conditioning is colour identity: a world draws uniformly among the
// lists whose colour identity covers every colour of the opponent's
// publicly seen cards, and among every list when none does. That is cheap
// (one precomputed list per colour mask) and keeps the believed pool in the
// right colours, which is most of what a limited deck's identity tells.
//
// A Belief is immutable after NewBelief and shared by every seat, world and
// goroutine of a process.
type Belief struct {
	u     *cards.Universe
	ord   map[string]int32 // primary-face name -> ordinal in u
	decks [][]int32        // each list's ordinals, ascending
	cover [32][]int32      // the lists whose colour identity covers mask m
	basic []bool           // per ordinal: a basic land (the pool's padding)
	blind []bool           // per ordinal: faceBlind on every face
}

// NewBelief builds a belief over decks. The card table is every distinct
// card of the corpus (by primary-face name, the first card of each name in
// name order), so it -- and every ordinal a closed world's HiddenIdentity
// event names -- is a function of the decks alone.
func NewBelief(decks [][]*cards.Card) (*Belief, error) {
	byName := make(map[string]*cards.Card)
	var names []string
	for _, d := range decks {
		for _, c := range d {
			n := primaryName(c)
			if n == "" {
				return nil, errors.New("searchprobe: a belief deck card has no primary face")
			}
			if _, ok := byName[n]; !ok {
				byName[n] = c
				names = append(names, n)
			}
		}
	}
	if len(names) == 0 {
		return nil, errors.New("searchprobe: an empty belief corpus")
	}
	sort.Strings(names)
	cs := make([]*cards.Card, len(names))
	b := &Belief{ord: make(map[string]int32, len(names)), basic: make([]bool, len(names)), blind: make([]bool, len(names))}
	for i, n := range names {
		c := byName[n]
		cs[i] = c
		b.ord[n] = int32(i)
		b.basic[i] = c.Faces[0].IsBasic() && c.Faces[0].IsLand()
		b.blind[i] = true
		for _, f := range c.Faces {
			if f != nil && !faceBlind(f) {
				b.blind[i] = false
			}
		}
	}
	b.u = cards.UniverseOf(cs)
	for _, d := range decks {
		ords := make([]int32, 0, len(d))
		var ident uint8
		for _, c := range d {
			ords = append(ords, b.ord[primaryName(c)])
			ident |= beliefColours(c)
		}
		if len(ords) == 0 {
			continue
		}
		sort.Slice(ords, func(i, j int) bool { return ords[i] < ords[j] })
		b.decks = append(b.decks, ords)
		for m := 0; m < 32; m++ {
			if uint8(m)&^ident == 0 {
				b.cover[m] = append(b.cover[m], int32(len(b.decks)-1))
			}
		}
	}
	if len(b.decks) == 0 {
		return nil, errors.New("searchprobe: an empty belief corpus")
	}
	return b, nil
}

// Decks is the number of lists in the corpus.
func (b *Belief) Decks() int { return len(b.decks) }

// Cards is the belief's card table, sorted by name: the world's
// state.Game.HypotheticalCards.
func (b *Belief) Cards() *cards.Universe { return b.u }

// Ord is the table ordinal of the card named name, if the corpus holds one.
func (b *Belief) Ord(name string) (int32, bool) {
	o, ok := b.ord[name]
	return o, ok
}

// draw picks one list for a world: uniform among the lists covering mask,
// else among all of them. The result is shared and read-only.
func (b *Belief) draw(mask uint8, r *rand.Rand) []int32 {
	pool := b.cover[mask&31]
	if len(pool) == 0 {
		return b.decks[r.IntN(len(b.decks))]
	}
	return b.decks[pool[r.IntN(len(pool))]]
}

// closedDeal fills dst with the identities one closed world deals a
// player's n unknown hidden cards: a list drawn by colour (draw), minus the
// cards already accounted for (seen: the player's publicly seen and pinned
// known cards that the table holds, ascending ordinals, as a multiset),
// shuffled; trimmed to n, or padded to n with the list's basic lands (its
// other cards when it has none), each pad drawn uniformly. It reads only
// its arguments and r.
func (b *Belief) closedDeal(mask uint8, seen []int32, n int, r *rand.Rand, dst []int32) []int32 {
	list := b.draw(mask, r)
	dst = dst[:0]
	j := 0
	for _, o := range list {
		for j < len(seen) && seen[j] < o {
			j++
		}
		if j < len(seen) && seen[j] == o {
			j++ // one seen copy accounts for one listed copy
			continue
		}
		dst = append(dst, o)
	}
	r.Shuffle(len(dst), func(i, k int) { dst[i], dst[k] = dst[k], dst[i] })
	if len(dst) >= n {
		return dst[:n]
	}
	basics := 0
	for _, o := range list {
		if b.basic[o] {
			basics++
		}
	}
	for len(dst) < n {
		if basics > 0 {
			k := r.IntN(basics)
			for _, o := range list {
				if b.basic[o] {
					if k == 0 {
						dst = append(dst, o)
						break
					}
					k--
				}
			}
		} else {
			dst = append(dst, list[r.IntN(len(list))])
		}
	}
	return dst
}

// Closed returns the public game as a seat with a closed view of its
// opponents' lists sees it: actor's own list stays, every other player's is
// dropped, and b is the belief its redeal draws them from. A nil b returns g
// unchanged (the open game).
func (g PublicGame) Closed(actor state.PlayerID, b *Belief) PublicGame {
	if b == nil {
		return g
	}
	decks := make([][]*cards.Card, len(g.Decks))
	if int(actor) < len(g.Decks) {
		decks[actor] = g.Decks[actor]
	}
	g.Decks = decks
	g.Belief = b
	return g
}

// beliefColours is the colours a card says its deck plays: its colour
// identity plus the colours it produces as a plain mana source (a basic
// Mountain's red, which colour identity leaves out). An "any colour" source
// says nothing.
func beliefColours(c *cards.Card) uint8 {
	if c == nil {
		return 0
	}
	m := c.ColourIdentity()
	for _, f := range c.Faces {
		if f == nil {
			continue
		}
		if mp := f.ManaProduction(); !mp.Any {
			for i := 0; i < 5; i++ {
				if mp.Colour[i] > 0 {
					m |= 1 << i
				}
			}
		}
	}
	return m
}

func primaryName(c *cards.Card) string {
	if c == nil || len(c.Faces) == 0 || c.Faces[0] == nil {
		return ""
	}
	return c.Faces[0].Name
}
