// Package mz is MageZero on gorge: the mage fork's AlphaZero-style MCTS
// (ComputerPlayerMCTS2, MCTSNode, MCTSPlayer), searching every decision one
// atomic pick at a time (bots/mz/atomic) on clones of the engine. It is a
// perfect-information seat (seat.EngineSeat). See mz-gorge-design.md and
// docs/mz/stage2-plan.md.
package mz

import "time"

// Options are the mage fork's search knobs (MCTSDefaults,
// Config.PlayerConfig) that offline search reads. The network's priors,
// noise and temperatures come with the encoder (stage 3).
type Options struct {
	// Budget is search_budget: the root visits, reused ones included, that
	// end a search.
	Budget int
	// Timeout is timeout_ms. A search ends at it once the tree holds a legal
	// priority node. Offline search is deterministic when it is long enough
	// that the budget always ends the search first.
	Timeout time.Duration
	// BackpropDiscount is backprop_discount: each level up the tree scales a
	// backed-up result by it.
	BackpropDiscount float64
	// PruneDuplicates is prune_duplicate_states.
	PruneDuplicates bool
	// ManualTap is manual_tapping. Both players in the sims tap this way.
	ManualTap bool
	// Mulligans is mulligans_enabled.
	Mulligans bool
}

// Defaults are MCTSDefaults.
func Defaults() Options {
	return Options{Budget: 1000, Timeout: 4 * time.Second, BackpropDiscount: 0.99}
}

const (
	cPUCT       = 1                 // ComputerPlayerMCTS.C_PUCT
	maxNodes    = 100000            // MAX_TREE_NODES
	maxDepth    = 1000              // MAX_TREE_DEPTH
	hardTimeout = 300 * time.Second // applyMCTS's forced time out
	maxNum      = 65                // makeChoiceAmount clamps max-min to 64
)
