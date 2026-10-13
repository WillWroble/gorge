package rules

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/rules/pay"
	"github.com/adams-shaun/gorge/state"
)

// paymentPlanReflectedAlt is the payment-plan census's alternative for a
// pay.PaymentPlanReflectedShape ability on source id (Exotic Orchard): the
// colours the resolution's own candidate walk names right now, as a
// choice-shaped alternative the planner expands into one concrete colour
// each (pay.PaymentPlanUnitAlternatives). ok is false when the ability is
// not that shape or reflects no colour.
func paymentPlanReflectedAlt(h effects.Host, p state.PlayerID, id state.ObjID, ma *cards.SA, amt int32) (windowManaAlt, bool) {
	if !pay.PaymentPlanReflectedShape(ma) {
		return windowManaAlt{}, false
	}
	var svars map[string]string
	if o := h.Game().Obj(id); o != nil && o.Face() != nil {
		svars = o.Face().SVars
	}
	ctx := effects.NewCtxPtr(id, p, effects.CtxInit{SVars: svars})
	counts, ok := pay.PaymentPlanReflectedCounts(effects.ManaReflectedCandidates(h, ctx, ma))
	if !ok {
		return windowManaAlt{}, false
	}
	return windowManaAlt{Ma: ma, Counts: counts, Amt: amt, Any: true}, true
}

// addPotentialReflected folds a pay.PaymentPlanReflectedShape ability into
// PotentialMana's bound as the colours the payment-plan census offers for it
// (each candidate colour at the ability's amount), and reports whether it
// did. The ability's text alone reads as a blank Produced$ -- the executor's
// colourless default for AB$ Mana -- which would drop every coloured cast
// only an Exotic Orchard pays from the planner's candidate walk.
func addPotentialReflected(h effects.Host, m *state.Mana, p state.PlayerID, id state.ObjID, ma *cards.SA) bool {
	alt, ok := paymentPlanReflectedAlt(h, p, id, ma, 1)
	if !ok {
		return false
	}
	for i, n := range alt.Mana() {
		m[i] = saturatingPotentialMana(m[i], n)
	}
	return true
}

// setPaymentPlanUnitAlt puts alt into source id's unit (units[idx], idx < 0:
// none yet): it replaces the alternative the shared census already listed
// for the same ability (its colourless stand-in for a reflected ability),
// else appends.
func setPaymentPlanUnitAlt(units []windowManaUnit, idx int, id state.ObjID, alt windowManaAlt) ([]windowManaUnit, int) {
	if idx < 0 {
		units = append(units, windowManaUnit{ID: id})
		idx = len(units) - 1
	}
	for k := range units[idx].Alts {
		if units[idx].Alts[k].Ma == alt.Ma {
			units[idx].Alts[k] = alt
			return units, idx
		}
	}
	units[idx].Alts = append(units[idx].Alts, alt)
	return units, idx
}
