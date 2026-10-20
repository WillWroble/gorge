package rules

import (
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// EnsurePotentialCasts adds to the pending priority decision every cast the
// potential-mana walk finds castable that it does not already offer: what
// the floating pool plus the seat's untapped sources can pay, not only what
// the pool pays now. Such a cast is announced and then paid in its CR 601.2g
// mana window, and reversed (CR 733.1) if it cannot be. This is the action
// set of an autotap player in mz-gorge (XMage offers whatever all available
// mana can pay, then its auto-tapper pays). The additions follow concede,
// so every existing index keeps its meaning. A driver calls it only for a
// seat.PotentialCastConsumer, the way it calls EnsurePaymentActions, so no
// other decision changes. It is a pure derived read and idempotent.
func (e *Engine) EnsurePotentialCasts() {
	d := e.pending
	if d == nil || d.Kind != decision.KPriority || d.PotentialCastsOffered {
		return
	}
	d.Options = e.withPotentialCasts(d.Player, d.Options)
	d.PotentialCastsOffered = true
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
