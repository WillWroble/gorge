package effects

import (
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

var castLedgerTriggeredControl = regexp.MustCompile(`ThisTurnCast_[^|]*\b(?:ControlledBy|OwnedBy) Triggered[A-Za-z]+`)

// pinnedCastLedgerTriggerCarriers is the set of corpus cards whose
// Count$ThisTurnCast_<spec> carries a Triggered* control referent, each with
// an end-to-end rules test proving the role reaches the cast-ledger filter
// (rules.TestMagebaneLizardCountsTheActivatorsNoncreatureCasts). The
// recognition census above proves only that the referent is understood; a
// new carrier here must come with such a test before it is added.
var pinnedCastLedgerTriggerCarriers = map[string]bool{
	"Magebane Lizard": true,
}

func TestCastLedgerTriggeredControlCarriersArePinned(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	found := map[string]bool{}
	seen := map[uintptr]bool{}
	for _, c := range reg.AllCards() {
		for _, f := range c.Faces {
			collectStrings(reflect.ValueOf(f), seen, func(s string) {
				if castLedgerTriggeredControl.MatchString(s) {
					found[f.Name] = true
				}
			})
		}
	}
	var names []string
	for n := range found {
		names = append(names, n)
		if !pinnedCastLedgerTriggerCarriers[n] {
			t.Errorf("%s: ThisTurnCast_ count with a Triggered* control referent has no end-to-end pin", n)
		}
	}
	sort.Strings(names)
	for n := range pinnedCastLedgerTriggerCarriers {
		if !found[n] {
			t.Errorf("pinned carrier %s no longer in the corpus (found %v); delete it", n, names)
		}
	}
}
