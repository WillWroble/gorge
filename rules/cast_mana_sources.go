package rules

import (
	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// Producing-source provenance for a cast spell (CR 106.3/601.2h: the mana a
// payment spends was produced by some source; "whenever you cast a ... spell
// using mana produced by CARDNAME" asks which). Tecutlan, the Searing Rift,
// The Myriad Pools and Barracks of the Thousand carry it as the SpellCast
// trigger's `ValidSA$ Spell.ManaFromCard.StrictlySelf`.
//
// The chain is the existing per-unit source attribution the TriggersWhenSpent$
// rider (Path of Ancestry) already rides:
//
//  1. production: a producer whose own face reads the identity
//     (effects.ManaSourceObserved) adds its units as a source-bearing,
//     empty-Valid pool batch (state.ManaRestriction.Source) -- effMana's
//     provenanceOnly arm and the plain-mana fast path both stamp it, so the
//     pool remembers which units that source made;
//  2. payment: emitRestrictedManaSpend consumes such batches first and
//     captures the consumed batches' sources (Session.ManaSpentSources),
//     deduplicated in deterministic batch order;
//  3. cast info: captureManaSources keeps that list on the pending cast when
//     one of its sources is observed, and manaSourcesCastInfo stamps it on a
//     trailing pay-time CastInfo (state.FlagManaSourcesSpent, IDs), which
//     events.Apply folds into Object.ManaSpentSources;
//  4. the reader: trigmatch's ValidSA$ arm admits Spell.ManaFromCard.
//     StrictlySelf when the trigger's own source is in that list.
//
// Everything is gated on an observing producer, so a game without one emits
// the same events as before (no head moves) and the payment path allocates
// nothing extra.

// captureManaSources copies the payment's spent-source capture onto pc when
// any captured source is an observed producer. It must run after the payment
// and before fireManaSpentTriggers consumes the capture.
func (e *Engine) captureManaSources(pc *pendingCast) {
	pc.manaSources = nil
	for _, src := range e.ManaSpentSources {
		if effects.ManaSourceObserved(e, src) {
			pc.manaSources = append([]state.ObjID(nil), e.ManaSpentSources...)
			return
		}
	}
}

// manaSourcesCastInfo builds the trailing CastInfo that carries the captured
// sources, or ok=false when none was captured. The flag is LOCAL (not ORed
// into the accumulating flags, the Convoked pattern), so no later CastInfo
// carries it and its fold arm's position is order-independent.
func manaSourcesCastInfo(pc *pendingCast, flags string) (events.Event, bool) {
	if len(pc.manaSources) == 0 {
		return events.Event{}, false
	}
	msFlags := events.FlagsString(events.FlagsFrom(flags) | state.FlagManaSourcesSpent)
	return events.Event{Kind: events.CastInfo, Obj: pc.card, Amount: int32(len(pc.manaSources)), Counter: msFlags,
		IDs: append([]state.ObjID(nil), pc.manaSources...)}, true
}
