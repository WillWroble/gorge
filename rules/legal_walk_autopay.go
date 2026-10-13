package rules

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/rules/pay"
	"github.com/adams-shaun/gorge/state"
)

// autoPayOffer reports whether the real-pool offer walk should offer the
// printed activated ability pile index idx of id although the floating pool
// cannot pay its cost: the planner has a witness that taps the player's mana
// sources for it, and the engine's own offer gate, priced with the pool plus
// that witness's production, confirms it (CR 602.2b: mana abilities may be
// activated while paying an activation's costs). beginActivation arms the
// same witness (armAbilityAutoPay), so the offer and the payment cannot drift.
//
// The potential walk (hyp != nil) already prices against the bound, so only
// the ordinary walk asks. Every non-mana part of the cost is read for real by
// the confirming gate, and the witness is planned so those parts stay payable.
func (w *legalWalk) autoPayOffer(p state.PlayerID, id state.ObjID, idx int, base Cost, ab *cards.SA) bool {
	if w.hyp != nil || !w.e.paidManaCensus || !abilityCostNeedsMana(base) || !pay.PaymentPlanPoolAccepted(asPayer(w.e), p) {
		return false
	}
	scope := abilityScope(ab)
	statics := w.costStatics.get()
	if w.autoBound == nil {
		b := w.e.PotentialMana(p)
		w.autoBound = &b
	}
	if *w.autoBound == w.e.G.Players[p].Pool {
		return false // nothing to tap: the pool is already the bound.
	}
	bound := *w.autoBound
	probe := base
	if !w.e.offerCastableUsing(statics, p, id, &probe, scope, true, &bound) {
		return false
	}
	out := w.e.planAbilityPayment(p, id, idx)
	if out.Plan == nil || out.Reason != "" || len(out.Plan.Activations) == 0 {
		return false
	}
	pool := w.e.G.Players[p].Pool
	for _, a := range out.Plan.Activations {
		for c, n := range a.Produces {
			pool[c] += int32(n)
		}
	}
	probe = base
	return w.e.offerCastableUsing(statics, p, id, &probe, scope, true, &pool)
}

// abilityCostNeedsMana reports whether c has a mana part a tap could pay.
func abilityCostNeedsMana(c Cost) bool {
	return c.Generic > 0 || c.Colored.Total() > 0
}

// armAbilityAutoPay gives the activation just begun (pc, the printed ability
// pile index idx of its source) the planner's witness when the floating pool
// cannot pay its mana: manaWindowAsk then taps the witness's sources one step
// at a time (revalidating each) instead of posing the manual window, exactly
// as a planned cast does. A shape the planner declines leaves the ordinary
// manual window in place.
func (e *Engine) armAbilityAutoPay(pc *pendingCast, idx int) {
	if pc == nil || pc.Payment != nil {
		return
	}
	mana := e.castPaymentMana(pc)
	if !abilityCostNeedsMana(mana) || !pay.PaymentPlanPoolAccepted(asPayer(e), pc.player) {
		return
	}
	if pay.CostPayableClassLife(asPayer(e), pc.player, paymentForCast(pc, mana),
		pipRider{AnyColor: pc.mayPlayIgnore, AnyType: pc.mayPlayIgnoreType}, mana, false) {
		return
	}
	out := e.planAbilityPayment(pc.player, pc.card, idx)
	if out.Plan == nil || out.Reason != "" || len(out.Plan.Activations) == 0 {
		return
	}
	pc.Payment = &plannedCastPayment{ActionID: decision.OptAutoFill, Plan: decision.ClonePaymentPlan(*out.Plan)}
	pc.PaymentNext = 0
}
