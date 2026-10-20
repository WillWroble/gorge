package rules

import (
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// OfferPotentialCasts sets whether p's priority decisions also offer every
// cast the potential-mana walk finds castable: what the floating pool plus
// p's untapped sources can pay, not only what the pool pays now. Such a cast
// is announced and then paid in its CR 601.2g mana window, and reversed (CR
// 733.1) if it cannot be. This is the action set of an autotap player in
// mz-gorge (XMage offers whatever all available mana can pay, then its
// auto-tapper pays). It is off for every seat by default, so no other
// decision stream changes.
func (e *Engine) OfferPotentialCasts(p state.PlayerID, on bool) {
	if on {
		e.potentialOffer |= 1 << p
	} else {
		e.potentialOffer &^= 1 << p
	}
}

// withPotentialCasts appends to p's offered options the potential walk's
// casts they lack, re-indexed after them. The input slice is not written.
func (e *Engine) withPotentialCasts(p state.PlayerID, opts []decision.Option) []decision.Option {
	_, cands := e.potentialWalkOf(p, false)
	out := opts[:len(opts):len(opts)]
	for _, c := range cands {
		if c.Kind != optCast || offersOption(out, c) {
			continue
		}
		c.Index = len(out)
		out = append(out, c)
	}
	return out
}

func offersOption(opts []decision.Option, c decision.Option) bool {
	for i := range opts {
		c.Index = opts[i].Index
		if optionEqual(&c, &opts[i]) {
			return true
		}
	}
	return false
}
