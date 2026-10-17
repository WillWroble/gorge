package host

import (
	"testing"
)

// The bench-only search policies read the engine a live table must never hand
// a seat (az's clairvoyant world clones the REAL engine, spec 2026-09-27 §1),
// so none of them may become a hosted opponent under any spelling of the
// factory. internal/archtest additionally forbids host from linking
// bots/azmcts at all.
//
// The real L10 teacher (`search`, BP-11) is the deliberate exception: it is a
// hosted policy ONLY when its package is linked into the binary, and it lives
// in four-yay now (bots/search), so gorge's host never links it: the factory
// refuses it here. four-yay bots/hosttest TestSearchIsHostedWhenLinked is the
// linked half (the factory accepts it and builds an EnvSeat + BoardSeat).
func TestHostedVocabularyRefusesSearchPolicies(t *testing.T) {
	for _, name := range []string{"az", "policynet", "legacy", "explore"} {
		if _, err := NormalizeBotPolicy(name); err == nil {
			t.Errorf("NormalizeBotPolicy(%q) accepted a bench-only search policy", name)
		}
		if _, err := NewBotPolicySeat(name, 1); err == nil {
			t.Errorf("NewBotPolicySeat(%q) built a seat", name)
		}
		if _, err := NewBotPolicySeatWithAutoPayMana(name, 1, true); err == nil {
			t.Errorf("NewBotPolicySeatWithAutoPayMana(%q) built a seat", name)
		}
	}

	// search is not linked into gorge (it moved to four-yay).
	const searchPolicy = "search"
	if _, err := NormalizeBotPolicy(searchPolicy); err == nil {
		t.Errorf("NormalizeBotPolicy(%q) accepted an unlinked policy", searchPolicy)
	}
	if _, err := NewBotPolicySeat(searchPolicy, 7); err == nil {
		t.Errorf("NewBotPolicySeat(%q) built a seat for an unlinked policy", searchPolicy)
	}
	if _, err := NewBotPolicySeatWithAutoPayMana(searchPolicy, 7, true); err == nil {
		t.Errorf("NewBotPolicySeatWithAutoPayMana(%q, auto-pay) built a seat for an unlinked policy", searchPolicy)
	}
}
