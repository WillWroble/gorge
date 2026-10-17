package rules

import "github.com/adams-shaun/gorge/engine/decision"

// triggerCostAskManaOnly reports whether d is the pay/decline ask of a live
// triggered "you may pay <cost>" window (engine/rules/cumulative.go's
// triggeredCostPaymentAsk) whose cost is mana only. A free function, not a
// method: the oracle runner holds a *Engine, so a method would grow
// engineSurface.
func triggerCostAskManaOnly(e *Engine, d *decision.Decision) bool {
	tc := e.triggerCost
	if tc == nil || d.Kind != decision.KChoose {
		return false
	}
	asks := false
	for _, o := range d.Options {
		if o.Kind == "trigger_cost_decline" {
			asks = true
		}
	}
	c := tc.announcedCost()
	return asks && c.HasManaPayment() && !c.HasNonMana()
}
