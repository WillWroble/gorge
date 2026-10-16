package botpolicy

// Detached is b without its incremental row cache (board_inc.go). The cache
// belongs to the Board buffer the host refills per decision, and it holds
// that buffer's engine lineage: the LIVE game and log. A host hands a seat
// b.Detached(), so the Board a seat holds reaches nothing of the live game
// -- in particular no opponent's hidden card -- beyond the fields BoardFromGame
// filled for it. The copy shares b's tables like any Board copy; only the
// cache pointer is dropped, so the host's own buffer keeps it.
func (b Board) Detached() Board {
	b.inc, b.incArmed = nil, false
	return b
}
