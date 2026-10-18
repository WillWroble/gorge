package cards

// AloneKeyword is CR 506.5's attack-alone / block-alone restriction family as
// a dense code. It is the ONE boundary conversion for the combat restriction
// reader (engine/rules/combat.ParseHiddenKeyword): these sentences are Forge
// script TEXT -- delivered as a printed K: line (canonicalised to the head
// below by canonicalKeywordHeads) and also as runtime keyword TEXT under an
// Animate HiddenKeywords$ parameter or a Pump/PumpAll KW$ -- so both
// spellings are matched here, once, and every downstream reader dispatches on
// the code (the codeshape no-strings ratchet).
type AloneKeyword uint16

// The vocabulary. 0 is any head outside it.
const (
	AloneKeywordNone AloneKeyword = iota
	// CR 506.5: "CARDNAME can't attack or block alone." -- the compound
	// restriction (Mogg Flunkies, Ember Beast, Wojek Bodyguard).
	AloneKeywordCantAttackOrBlockAlone
	// CR 506.5: "CARDNAME can't attack alone." (Bonded Construct, Militia
	// Rallier, Raging Kronch, Trusty Companion).
	AloneKeywordCantAttackAlone
	// CR 506.5: "CARDNAME can't block alone." (Craven Hulk).
	AloneKeywordCantBlockAlone
	// AloneKeywordCount is the vocabulary size (a table bound, not a value).
	AloneKeywordCount
)

// aloneKeywordNames is the canonical spelling each code answers to, indexed
// by code; the table below ALSO keys each code by its printed Forge sentence
// ("CARDNAME can't attack alone."), which reaches this reader from a runtime
// Animate/Pump grant and never passes through the K: parser.
var aloneKeywordNames = [...]string{
	AloneKeywordNone:                   "",
	AloneKeywordCantAttackOrBlockAlone: "CantAttackOrBlockAlone",
	AloneKeywordCantAttackAlone:        "CantAttackAlone",
	AloneKeywordCantBlockAlone:         "CantBlockAlone",
}

// aloneKeywordHeadCodes resolves one already-lower-cased, trimmed head to its
// code. Built once at package init, so the per-declaration read is a hash
// probe rather than a run of case-folding string compares.
var aloneKeywordHeadCodes = NewStrCodes(
	StrEntry[AloneKeyword]{Key: "cantattackorblockalone", Val: AloneKeywordCantAttackOrBlockAlone},
	StrEntry[AloneKeyword]{Key: "cardname can't attack or block alone.", Val: AloneKeywordCantAttackOrBlockAlone},
	StrEntry[AloneKeyword]{Key: "cantattackalone", Val: AloneKeywordCantAttackAlone},
	StrEntry[AloneKeyword]{Key: "cardname can't attack alone.", Val: AloneKeywordCantAttackAlone},
	StrEntry[AloneKeyword]{Key: "cantblockalone", Val: AloneKeywordCantBlockAlone},
	StrEntry[AloneKeyword]{Key: "cardname can't block alone.", Val: AloneKeywordCantBlockAlone},
)

// AloneKeywordOf resolves one keyword head (already passed through
// KeywordHead, with any "HIDDEN " marker stripped by the caller) to its
// AloneKeyword, case-insensitively, or AloneKeywordNone.
func AloneKeywordOf(head string) AloneKeyword {
	if head == "" {
		return AloneKeywordNone
	}
	return aloneKeywordHeadCodes.Code(lowerASCIIString(head))
}

// lowerASCIIString lower-cases s for the head tables. The Forge spellings
// folded here are ASCII sentences and heads, so a byte-wise fold is exact;
// the head reader pays it once per keyword line read.
func lowerASCIIString(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; 'A' <= c && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				b[j] = lowerASCII(b[j])
			}
			return string(b)
		}
	}
	return s
}
