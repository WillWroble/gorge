package rules

import (
	"fmt"
	"testing"
)

// TestPriorityOptionsWithPaidManaCensus: the probe re-derives the pending
// priority window under either flag value on a clone -- the pool-short ability
// is in the list exactly when the probe's flag is on, whatever the engine's
// own setting, and the live engine's pending options are not disturbed.
func TestPriorityOptionsWithPaidManaCensus(t *testing.T) {
	t.Parallel()
	for _, c := range pmcClasses() {
		for _, engineFlag := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/engine=%v", c.name, engineFlag), func(t *testing.T) {
				e, d, sage, _ := pmcPriority(t, engineFlag, c, false, pmcSage, false)
				if _, ok := autopayAbilityOffer(d, sage); ok != engineFlag {
					t.Fatalf("precondition: engine flag=%v but ability offered=%v: %s", engineFlag, ok, pmcKinds(d))
				}
				before := len(d.Options)
				for _, on := range []bool{false, true} {
					got := e.PriorityOptionsWithPaidManaCensus(on)
					found := false
					for _, o := range got {
						if o.Kind == "ability" && o.Obj == sage {
							found = true
						}
					}
					if found != on {
						t.Fatalf("probe(on=%v): ability offered=%v, want %v (%d options)", on, found, on, len(got))
					}
				}
				if e.paidManaCensus != engineFlag || len(e.Pending().Options) != before {
					t.Fatalf("probe disturbed the engine: flag=%v options %d -> %d", e.paidManaCensus, before, len(e.Pending().Options))
				}
			})
		}
	}
}
