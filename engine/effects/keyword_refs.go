package effects

import "github.com/adams-shaun/gorge/engine/cards"

// Keyword heads this package asks a face about by name, compiled once
// (cards.NewKeywordRef) so a check is a mask or bitset test, not a string
// switch and keyword-line scan.
var (
	kwRefAffinity   = cards.NewKeywordRef("Affinity")
	kwRefChangeling = cards.NewKeywordRef("Changeling")
	kwRefDevoid     = cards.NewKeywordRef("Devoid")
	kwRefSuspend    = cards.NewKeywordRef("Suspend")
)
