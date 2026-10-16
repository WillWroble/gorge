package policynet

import (
	"strings"
	"unicode/utf8"
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

// cardTypeBits-indexed masks (bit i = cardTypeBits[i]).
const (
	tbLand uint16 = 1 << iota
	tbCreature
	tbArtifact
	tbEnchantment
	tbInstant
	tbSorcery
	tbPlaneswalker
	tbBattle
	tbLegendary
	tbBasic
)

// typeMasks folds a type line into two cardTypeBits masks: fold has bit i
// when some word equals cardTypeBits[i] case-insensitively (hasTypeWord's
// test), exact when some word equals it exactly.
func typeMasks(types string) (fold, exact uint16) {
	add := func(w string) {
		for i, t := range cardTypeBits {
			if w == t {
				exact |= 1 << i
				fold |= 1 << i
			} else if strings.EqualFold(w, t) {
				fold |= 1 << i
			}
		}
	}
	if !isASCII(types) {
		for _, w := range strings.Fields(types) {
			add(w)
		}
		return
	}
	for i := 0; ; {
		w, next := nextField(types, i)
		if w == "" {
			return
		}
		add(w)
		i = next
	}
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
