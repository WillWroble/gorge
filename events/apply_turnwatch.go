package events

import "github.com/adams-shaun/gorge/state"

// apply_turnwatch.go holds the per-seat turn-watcher tallies on state.Player
// (SpellsCastThisTurn, LifeGainedThisTurn, LifeLostThisTurn,
// TokensCreatedThisTurn). They are written only here and from the folds that
// already own the underlying state change (foldLifeChange, foldDamage, the
// token mints), and cleared in foldTurnChange, so a log-only replay derives
// them and no direct state write exists outside Apply.

// resetTurnWatchers clears one seat's watchers at TurnChange.
func resetTurnWatchers(p *state.Player) {
	p.SpellsCastThisTurn = 0
	p.LifeGainedThisTurn = 0
	p.LifeLostThisTurn = 0
	p.TokensCreatedThisTurn = 0
}

// tallySpellCast counts one spell put on the stack by e.Player (the same
// PutOnStack classification Engine.SpellsCastThisTurnBy scans the log for).
func tallySpellCast(g *state.Game, e *Event) {
	if validPlayer(g, e.Player) {
		g.Players[e.Player].SpellsCastThisTurn++
	}
}

// tallyTokenCreated counts one token minted under p's control by a
// TokenCreate, CardToken, CopyToken or MyriadCopy fold.
func tallyTokenCreated(g *state.Game, p state.PlayerID) {
	if validPlayer(g, p) {
		g.Players[p].TokensCreatedThisTurn++
	}
}
