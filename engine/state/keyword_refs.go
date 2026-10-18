package state

import "github.com/adams-shaun/gorge/engine/cards"

// Keyword heads this package asks a face about by name, compiled once
// (cards.NewKeywordRef) so a check is a mask or bitset test, not a string
// switch and keyword-line scan.
var (
	kwRefBestow      = cards.NewKeywordRef("Bestow")
	kwRefReconfigure = cards.NewKeywordRef("Reconfigure")
)
