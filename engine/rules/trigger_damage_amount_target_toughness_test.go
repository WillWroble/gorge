package rules

import "testing"

// DamageAmount$ EQTargetToughness (Taii Wakeen, Perfect Shot) compares the
// damage to the damaged creature's toughness; it must not fail closed.
func TestDamageDoneDamageAmountEQTargetToughness(t *testing.T) {
	t.Parallel()
	e, id := damageAmountBoard(t, "DamageDone", "DamageAmount$ EQTargetToughness")
	if got := e.Toughness(id); got != 9 {
		t.Fatalf("precondition: carrier toughness = %d, want 9", got)
	}
	if damageAmountFires(t, e, id, 2) {
		t.Fatal("2 damage to a toughness-9 creature triggered EQTargetToughness")
	}
	if !damageAmountFires(t, e, id, 9) {
		t.Fatal("9 damage to a toughness-9 creature did not trigger EQTargetToughness")
	}
}
