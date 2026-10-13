package rules

import (
	"github.com/adams-shaun/gorge/events"
)

// proposeDrawBatchReplacement holds one draw instruction's batch through the
// DrawCards replacement class before any card moves (Forge's
// Player.drawCards(n) DrawCards pass; XMage's DRAW_TWO_OR_MORE_CARDS). It is
// the DrawBatchProposal arm of CountReplacementProposed -- no new Host method
// exists for it because the held-proposal seam already carries a count.
//
// The proposal is never logged. A consumed batch reports Amount 0: a
// replacement's body drew in the caller's place, so the caller draws nothing
// for it (effects' proposeDrawBatch reads exactly that). A batch proposed
// from inside a draw-replacement body re-enters with the lineage's applied
// set excluded (CR 614.5/616.1e), which is what keeps Thought Reflection's
// own two-card body from re-matching Thought Reflection while Alms Collector
// still replaces it. A draw instruction emitted by a replacement body of any
// OTHER class stays suppressed exactly as before (the applyingReplacement
// guard), because no draw lineage is in flight to exclude.
func proposeDrawBatchReplacement(e *Engine, ev events.Event) events.Event {
	if e.applyingReplacement && len(e.replDrawChain) == 0 {
		return ev
	}
	saved := e.replExclude
	if len(e.replDrawChain) > 0 {
		e.replExclude = e.replDrawChain
	}
	replaced, handled := e.applyReplacements(ev)
	e.replExclude = saved
	if handled {
		replaced.Amount = 0
	}
	return replaced
}
