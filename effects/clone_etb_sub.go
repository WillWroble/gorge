package effects

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/state"
)

// The as-enters copy's "except it enters with N additional +1/+1 counters"
// rider (Altered Ego: `DB$ Clone | ... | SubAbility$ DBAddCounter` with
// `DBAddCounter:DB$ PutCounter | Defined$ Self | CounterType$ P1P1 | ETB$ True
// | CounterNum$ X`). The rider is part of the copy election, not of the
// entry: CR 707.9 makes it an exception of the copy effect, and CR 614.1c
// makes the whole "you may have it enter as a copy" replacement optional,
// so a declined election (or one with nothing to copy) enters the object as
// itself with NO extra counters.

// cloneETBCounterSubKeys are the keys an admitted rider link may carry.
var cloneETBCounterSubKeys = state.NewNameSet(
	"Defined", "CounterType", "ETB", "CounterNum", "SubAbility",
)

// CloneETBSubChainOK reports whether an as-enters Clone body's SubAbility$
// chain lies inside the implemented rider shape: every link an unconditional
// `DB$ PutCounter | Defined$ Self | ETB$ True` with no asking modifier. A body
// with no SubAbility$ is trivially inside it. Any other chain (Moritte of the
// Frost's DB$ Effect replacement, Undercover Operative's ConditionPresent$
// shield counter) fails closed, keeping the body out of the ETB election.
// The key scan only answers a boolean, so map order cannot reach an event.
func CloneETBSubChainOK(sa *cards.SA) bool {
	if sa == nil {
		return false
	}
	if _, has := sa.Param(cards.PKSubAbility); !has {
		return true
	}
	if sa.Sub == nil {
		// An unlinked chain cannot be classified.
		return false
	}
	for link, d := sa.Sub, 0; link != nil; link, d = link.Sub, d+1 {
		if d >= maxChain || !IsPutCounter(link) {
			return false
		}
		for k := range link.Params {
			if !cloneETBCounterSubKeys.Has(k) {
				return false
			}
		}
		pc := PutCounterOf(link)
		if !pc.ETB || pc.EntryFoldBlocked || len(pc.Unread) > 0 || DefinedOf(link).Defined.Text != "Self" {
			return false
		}
		if _, has := link.Param(cards.PKSubAbility); has && link.Sub == nil {
			return false
		}
	}
	return true
}

// CloneETBNoCopy reports that an as-enters copy election was recorded on the
// entering object and no copy will be made: the controller declined ("Enter
// as itself", also the only option with nothing to copy), or the chosen
// template is no longer a legal copy source when the replacement applies.
// The optional replacement is then not applied at all (CR 614.1c), so its
// SubAbility$ rider -- the copy's "enters with additional counters"
// exception -- must not run either. False when no election was recorded:
// that path keeps effClone's loud unimplemented-API fallback.
func CloneETBNoCopy(g *state.Game, c *Ctx, sa *cards.SA) bool {
	if c == nil || sa == nil || !c.CloneEnter.ETB || !c.CloneEnter.ChoiceValid || !isCloneSA(sa) {
		return false
	}
	return c.CloneEnter.Choice == 0 || !cloneETBTemplateLegal(g, c, CloneOf(sa))
}
