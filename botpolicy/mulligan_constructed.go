package botpolicy

import "github.com/adams-shaun/gorge/decision"

// MulliganConstructed is the opt-in rule for constructed (60-card) decks:
// MulliganLands' shape with the land window derived from the deck's own land
// share (Board.OwnDeck, the seat's own genesis manifest) and a "has an early
// play" test on the seven. It consumes no rng and reads only the deciding
// seat's own hand and own deck list.
//
// With K = hand - bottom cards to keep:
//
//   - K >= 6 (the opening seven, or one mulligan): keep when the seven hold
//     2..hi lands, at least one spell castable by turn 3 with those lands
//     (its mana value at most min(lands, 3), every coloured pip covered by a
//     producing land), and the lands produce a colour some spell needs.
//     hi is 5, or 4 for a land-light deck (see landWindow).
//   - K == 5: keep unless the seven hold no land or no spell (as
//     MulliganLands).
//   - K <= 4: always keep.
//
// Bottoming is MulliganLands' (landBottoming). A hand the census cannot read
// is kept.
const MulliganConstructed MulliganRule = MulliganNever + 1

// landWindow is the land-count window of a 7-card keep: lo is 2 and hi is
// ceil(7*share)+2 clamped to [4, 5], share being the own deck's land fraction
// (24 of 60 gives 5, 16 of 60 gives 4). Without a manifest the window is
// 2..5, the fixed Limited window.
func (b Board) landWindow() (lo, hi int) {
	lo, hi = 2, 5
	m := b.OwnDeck
	if m == nil {
		return lo, hi
	}
	var total, lands int
	for _, r := range m.Main {
		total += r.Count
		if r.Land {
			lands += r.Count
		}
	}
	if total <= 0 {
		return lo, hi
	}
	// ceil(7*lands/total) in integers.
	exp := (7*lands + total - 1) / total
	hi = min(max(exp+2, 4), 5)
	return lo, hi
}

// keepConstructed is MulliganConstructed's keep/mulligan rule for a hand
// that will bottom `bottom` cards if kept.
func (b Board) keepConstructed(bottom int) bool {
	hand, lands, ok := b.handLands()
	if !ok {
		return true
	}
	switch k := hand - bottom; {
	case k >= 6:
		lo, hi := b.landWindow()
		return lands >= lo && lands <= hi && b.handHasEarlyPlay()
	case k == 5:
		return lands >= 1 && lands < hand
	default:
		return true
	}
}

// producesColour reports whether land card c can make colour (an index of
// the five colours): an "any" producer and a land with no recorded
// production (Evolving Wilds and the like) count for every colour, the same
// reading landBottoming uses.
func producesColour(c Card, colour int) bool {
	pr := c.Produces
	if pr.Any {
		return true
	}
	none := true
	for _, v := range pr.Colour {
		if v != 0 {
			none = false
			break
		}
	}
	return none || pr.Colour[colour] > 0
}

// handHasEarlyPlay reports whether the hand holds a spell castable by turn 3
// with its lands AND its lands produce a colour some spell needs. A hand of
// only colourless spells has no needed colour, so that half is vacuous.
func (b Board) handHasEarlyPlay() bool {
	var lands, spells []Card
	for _, c := range b.Cards.All() {
		if !c.Castable || c.OnBattlefield {
			continue
		}
		if isLandCard(c) {
			lands = append(lands, c)
		} else {
			spells = append(spells, c)
		}
	}
	avail := min(len(lands), 3)
	if avail == 0 {
		return false
	}
	var src [5]int
	for _, l := range lands {
		for col := range src {
			if producesColour(l, col) {
				src[col]++
			}
		}
	}
	var needed [5]bool
	castable := false
	for _, s := range spells {
		pips := colourPips(s.ManaCost)
		coloured := 0
		ok := s.CMC <= int32(avail)
		for col := range pips {
			if pips[col] > 0 {
				needed[col] = true
			}
			coloured += int(pips[col])
			if int(pips[col]) > min(src[col], avail) {
				ok = false
			}
		}
		if coloured > avail {
			ok = false
		}
		castable = castable || ok
	}
	if !castable {
		return false
	}
	anyNeeded := false
	for col := range needed {
		if needed[col] {
			anyNeeded = true
			if src[col] > 0 {
				return true
			}
		}
	}
	return !anyNeeded
}

// MulliganAnswer answers a KMulligan ask (either shape) under rule from the
// deciding seat's own Board, for a seat decorator that is not a
// botpolicy-driven Bot (the sb-tactical hosted seat). ok is false for
// MulliganCoin, an empty decision, or a shape the rule leaves to the seat's
// own policy. It consumes no rng.
func MulliganAnswer(b Board, rule MulliganRule, d *decision.Decision) (choices []int, ok bool) {
	b.Mulligan = rule
	return b.mulliganByRule(d)
}
