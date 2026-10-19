package policynet

import (
	"strings"
	"unicode/utf8"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/view"
)

// Type-line reads without strings.Fields' per-call allocation. A type line
// is space-separated words; every reader here sees exactly the words
// strings.Fields would return, in order: an ASCII line is split in place on
// the six ASCII whitespace bytes (the only whitespace an ASCII string can
// hold), and a line with any non-ASCII byte falls back to strings.Fields.

// nextField returns the first field of s at or after byte i and the index
// just past it; word is "" (and next == len(s)) when none is left. s must be
// ASCII.
func nextField(s string, i int) (word string, next int) {
	for i < len(s) && isSpaceByte(s[i]) {
		i++
	}
	start := i
	for i < len(s) && !isSpaceByte(s[i]) {
		i++
	}
	return s[start:i], i
}

// isASCII reports whether s has no byte >= utf8.RuneSelf.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// cardTypeMasks are cardTypeBits as cards.TypeMask bits:
// cardTypeMasks[i] is cardTypeBits[i]'s bit.
var cardTypeMasks = [len(cardTypeBits)]cards.TypeMask{
	cards.TypeLand, cards.TypeCreature, cards.TypeArtifact, cards.TypeEnchantment, cards.TypeInstant,
	cards.TypeSorcery, cards.TypePlaneswalker, cards.TypeBattle, cards.TypeLegendary, cards.TypeBasic,
}

// typeMasks folds a card's type line into two cards.TypeMasks: fold has a
// type's bit when some word equals its name case-insensitively, exact when
// some word equals it exactly (cards.TypeLineMasks). A projected card
// answers from its compiled facts; a decoded or hand-built one parses Types.
func typeMasks(cv *view.CardView) (fold, exact cards.TypeMask) {
	if cv.Facts.OK {
		return cv.Facts.TypeFold, cv.Facts.TypeExact
	}
	return cards.TypeLineMasks(cv.Types)
}

// hasType reports whether some word of cv's type line is t's name
// (case-insensitive).
func hasType(cv *view.CardView, t cards.TypeMask) bool {
	fold, _ := typeMasks(cv)
	return fold&t != 0
}

// cardManaValue is manaCostBits(cv.ManaCost)'s mana value, from the
// compiled facts when the projection set them.
func cardManaValue(cv *view.CardView) int {
	if cv.Facts.OK {
		return cv.Facts.ManaValue
	}
	return mvOf(cv.ManaCost)
}

// eachField calls f with each of strings.Fields(s)'s words, in order.
func eachField(s string, f func(string)) {
	if !isASCII(s) {
		for _, w := range strings.Fields(s) {
			f(w)
		}
		return
	}
	for i := 0; ; {
		w, next := nextField(s, i)
		if w == "" {
			return
		}
		f(w)
		i = next
	}
}
