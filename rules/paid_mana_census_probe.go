package rules

import "github.com/adams-shaun/gorge/decision"

// PriorityOptionsWithPaidManaCensus is the A/B probe for the paid-cost census
// (paid_mana_census.go, disabled by Config.DisablePaidManaCensus): the option
// list the pending priority decision's player would be offered with the census
// on or off, whatever the engine's own setting. It walks a clone, so the engine, its memos and its log are not
// touched; call it at an intent boundary only (as Clone requires). nil when no
// priority decision is pending. Measurement code reads it; no rule does.
func (e *Engine) PriorityOptionsWithPaidManaCensus(on bool) []decision.Option {
	d := e.pending
	if d == nil || d.Kind != decision.KPriority {
		return nil
	}
	c := e.Clone()
	c.paidManaCensus = on
	return c.legalActionsWithWindow(d.Player, nil)
}
