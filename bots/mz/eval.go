package mz

import (
	"math"

	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
)

// GameStateEvaluator3's weights.
const (
	lifeValue     = 0.6
	handCardValue = 1.0
	permBaseValue = 1.0
	normalization = 15.0
)

// evaluate is GameStateEvaluator3.evaluateNormalized: tanh of the resource
// difference from p's side, ±1 once the game is won or lost.
func evaluate(e *rules.Engine, p state.PlayerID) float64 {
	if e.G.Over && !e.G.Draw {
		if e.G.Winner == p {
			return 1
		}
		return -1
	}
	var mine, theirs float64
	for i := range e.G.Players {
		q := state.PlayerID(i)
		if q == p {
			mine = resources(e, q)
		} else {
			theirs = resources(e, q)
		}
	}
	return math.Tanh((mine - theirs) / normalization)
}

// resources is life·0.6, plus the harmonic number of the hand size, plus
// 1 + mana value for each permanent p controls.
func resources(e *rules.Engine, p state.PlayerID) float64 {
	s := float64(max(0, e.G.Players[p].Life)) * lifeValue
	for i := range len(e.G.Zone(state.ZHand, p)) {
		s += handCardValue / float64(i+1)
	}
	for i := range e.G.Players {
		for _, id := range e.G.Zone(state.ZBattlefield, state.PlayerID(i)) {
			if o := e.G.Obj(id); o != nil && o.Controller == p && !o.PhasedOut {
				s += permBaseValue
				if f := o.Face(); f != nil {
					s += float64(f.ManaValue())
				}
			}
		}
	}
	return s
}
