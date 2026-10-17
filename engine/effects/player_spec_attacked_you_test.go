package effects

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// TestPlayerSpecAttackedYouTheirCurrentTurn pins Forge's
// attackedYouTheirCurrentTurn / attackedYouCtrlTheirCurrentTurn_Planeswalker
// player properties (Sandswirl Wanderglyph's Caster$, CR 508.1b): the
// candidate is the active player AND declared an attack this turn at `you`
// itself, or at a planeswalker `you` controls. Before this the qualifier was
// unknown and failed closed, so the restriction applied to nobody.
func TestPlayerSpecAttackedYouTheirCurrentTurn(t *testing.T) {
	const spec = "Player.Opponent+attackedYouTheirCurrentTurn,Player.Opponent+attackedYouCtrlTheirCurrentTurn_Planeswalker"
	g := state.NewGame([]string{"a", "b", "c"})
	g.Active = 1
	pc := PlayerSpecCtx{}
	if MatchesPlayerSpecCtx(g, spec, 1, 0, pc) {
		t.Fatal("seat 1 matched before attacking anyone")
	}
	g.Players[1].AttackedPlayersThisTurn = 1 << 0
	if !MatchesPlayerSpecCtx(g, spec, 1, 0, pc) {
		t.Fatal("seat 1 attacked seat 0 on its turn, want a match")
	}
	if MatchesPlayerSpecCtx(g, spec, 1, 2, pc) {
		t.Fatal("seat 1 attacked seat 0, not seat 2")
	}
	g.Players[1].AttackedPlayersThisTurn = 0
	g.Players[1].AttackedPlaneswalkersThisTurn = 1 << 0
	if !MatchesPlayerSpecCtx(g, spec, 1, 0, pc) {
		t.Fatal("seat 1 attacked a planeswalker seat 0 controls, want the planeswalker alternative to match")
	}
	if MatchesPlayerSpecCtx(g, "Player.Opponent+attackedYouTheirCurrentTurn", 1, 0, pc) {
		t.Fatal("a planeswalker attack matched the player-only qualifier")
	}
	// "Their current turn": the watcher of a seat that is not active never
	// matches, even when its bits are set.
	g.Active = 2
	if MatchesPlayerSpecCtx(g, spec, 1, 0, pc) {
		t.Fatal("seat 1 matched on seat 2's turn")
	}
}
