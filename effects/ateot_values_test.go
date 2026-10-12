package effects

import (
	"sort"
	"testing"

	"github.com/adams-shaun/gorge/cards"
)

// TestAtEOTValuesMatchCards keeps cards.AtEOTValues (what `make report`
// treats as supported) identical to the riders effects actually schedules.
func TestAtEOTValuesMatchCards(t *testing.T) {
	vals := cards.AtEOTValues()
	sort.Strings(vals)
	if len(vals) == 0 {
		t.Fatal("no AtEOT values")
	}
	for _, v := range vals {
		if _, ok := atEOTRiders.Get(v); !ok {
			t.Errorf("cards lists AtEOT$ %s but effects has no rider", v)
		}
	}
	for _, v := range []string{"ExileCombat", "SacrificeCombat", "Exile", "Sacrifice", "Hand", "Destroy"} {
		if !cards.AtEOTValueSupported(v) {
			t.Errorf("cards does not list implemented AtEOT$ %s", v)
		}
		if r, ok := atEOTRiders.Get(v); !ok || r.body == "" {
			t.Errorf("effects has no rider for %s", v)
		}
	}
	if cards.AtEOTValueSupported("Library") {
		t.Error("Library is listed supported but has no rider")
	}
	if _, ok := atEOTRiders.Get("Library"); ok {
		t.Error("Library has a rider but cards does not list it")
	}
}
