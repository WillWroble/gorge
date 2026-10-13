package rules

import "github.com/adams-shaun/gorge/cards"

// oracleNameUniverse is the card-name universe an oracle scenario's NameCard
// asks range over: the distinct cards the scenario seats (every deck card,
// the filler included, and every sideboard card), in seat-then-deck order.
//
// Without one, a name ask falls back to the pre-universe builders: the
// as-enters ask lists only PUBLIC objects matching the filter and, when none
// does, names the top of the library -- the Wastes filler, a LAND, even for
// Skyseer's Chariot's "choose a nonland card name" -- and the mid-resolution
// ask always names the library top. Those answers ignore the card's own
// ValidCards$ and are not legal answers in XMage, whose name dialog then
// consumes the next scripted choice. A scenario-scoped universe makes the
// ask filter exactly as a production game's (corpus-backed) ask does, while
// every offered name is a card the scenario seats -- a name the generator can
// script and XMage knows.
//
// It is nil (no universe, the scenario plays exactly as before) unless a
// seated card mentions a name choice (cards.MentionsNameChoice): per
// cards.NeedsFullNameUniverse's contract, the universe reaches no other
// game, and the land-type vocabulary keeps reading NamedCorpus.
func oracleNameUniverse(decks, sideboards [][]*cards.Card) *cards.Universe {
	need := false
	seen := map[*cards.Card]bool{}
	var cs []*cards.Card
	for _, group := range [][][]*cards.Card{decks, sideboards} {
		for _, deck := range group {
			for _, c := range deck {
				if c == nil || seen[c] {
					continue
				}
				seen[c] = true
				cs = append(cs, c)
				need = need || cards.MentionsNameChoice(c)
			}
		}
	}
	if !need {
		return nil
	}
	return cards.UniverseOf(cs)
}
