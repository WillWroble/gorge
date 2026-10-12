package templates_test

import (
	"strings"
	"testing"
)

// TestStaticG7NamedGaps: a level-B static row the G7 slice measured and could
// not serve carries its own measured reason instead of the generic "effect not
// observable on a probe or the card".
//
//   - (The Lunar Whale static#0.1 is served since lb-setupparity: the
//     attackedThisTurn predicate is evaluated, and the offer observation
//     crews and attacks first -- static_offer_self_attack_test.go.)
//   - Emet-Selch, Unsundered static#1.0 (Hades, Sorcerer of Eld's graveyard
//     MayPlay$): setup places the graveyard probe through the card's own
//     "exile instead of graveyard" replacement, so the probe ends in exile.
func TestStaticG7NamedGaps(t *testing.T) {
	for _, row := range []struct{ card, key, want string }{
		{"Emet-Selch, Unsundered", "static#1.0", "exiled by the card's own Moved replacement"},
	} {
		got := staticSkipReason(t, row.card, row.key)
		if !strings.Contains(got, row.want) {
			t.Errorf("%s %s skip = %q, want it to contain %q", row.card, row.key, got, row.want)
		}
		if strings.Contains(got, "not observable on a probe or the card") {
			t.Errorf("%s %s still carries the generic reason: %q", row.card, row.key, got)
		}
	}
}
