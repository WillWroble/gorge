package cards

import (
	"reflect"
	"testing"
)

// The prevention-rider coverage gate: a PreventDamage ability whose
// PreventionSubAbility$ rider this engine does not resolve (cards.
// PreventionRiderResolved) must name api:PreventDamage.OtherRider, so a card
// whose shield prevents silently — never running its printed follow-up — is
// counted UNSUPPORTED instead of playable-with-a-loud-Note. A rider the
// engine resolves (DealDamage/PutCounter/GainLife over a bound
// ShieldEffectTarget$ scope) must NOT add the synthetic.
func TestPreventionRiderCoverageSynthetic(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		src  string
		want bool
	}{
		{"putcounter rider, Targeted", `Name:Faith Probe
ManaCost:1 W
Types:Instant
A:SP$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Rider | ShieldEffectTarget$ Targeted
SVar:Rider:DB$ PutCounter | Defined$ ShieldEffectTarget | CounterType$ P1P1 | CounterNum$ PreventedDamage
Oracle:x
`, false},
		{"gainlife rider, You", `Name:Glow Probe
ManaCost:1 W
Types:Instant
A:SP$ PreventDamage | ValidTgts$ Any | Amount$ 3 | PreventionSubAbility$ Rider | ShieldEffectTarget$ You
SVar:Rider:DB$ GainLife | Defined$ ShieldEffectTarget | LifeAmount$ PreventedDamage
Oracle:x
`, false},
		{"dealdamage rider, ParentTarget", `Name:Retribution Probe
ManaCost:1 W W
Types:Instant
A:SP$ Pump | SubAbility$ Prev
SVar:Prev:DB$ PreventDamage | ValidTgts$ Creature | Amount$ X | PreventionSubAbility$ Deal | ShieldEffectTarget$ ParentTarget
SVar:Deal:DB$ DealDamage | Defined$ ShieldEffectTarget | NumDmg$ PreventedDamage
SVar:X:Count$ChosenNumber
Oracle:x
`, false},
		{"no rider at all", `Name:Plain Shield
ManaCost:W
Types:Instant
A:SP$ PreventDamage | ValidTgts$ Creature | Amount$ 2
Oracle:x
`, false},
		{"rider API outside the resolved set", `Name:Effect Rider Probe
ManaCost:1 W
Types:Instant
A:SP$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Rider | ShieldEffectTarget$ Targeted
SVar:Rider:DB$ Effect | RememberObjects$ ShieldEffectTarget | Triggers$ DelTrig
Oracle:x
`, true},
		{"rider SVar unresolved", `Name:Missing Rider Probe
ManaCost:1 W
Types:Instant
A:SP$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Nowhere
SVar:Other:DB$ PutCounter | CounterType$ P1P1
Oracle:x
`, true},
		{"scope outside the bound set", `Name:Bad Scope Probe
ManaCost:1 W
Types:Instant
A:SP$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Rider | ShieldEffectTarget$ Enchanted
SVar:Rider:DB$ PutCounter | CounterType$ P1P1 | CounterNum$ PreventedDamage
Oracle:x
`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ParseBytes("probe.txt", []byte(tc.src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			c.Link()
			got := c.Primitives()
			has := false
			for _, p := range got {
				if p == "api:PreventDamage.OtherRider" {
					has = true
				}
			}
			if has != tc.want {
				t.Fatalf("Primitives() = %v, OtherRider present = %v, want %v", got, has, tc.want)
			}
			if tc.want && len(got) == 0 {
				t.Fatal("vacuous: card parsed to no primitives at all")
			}
		})
	}
}

// The predicate itself is exercised directly on the sub-SVar resolution so a
// regression in ResolveSVar reachability (the rider body no longer resolving)
// fails here rather than only through Primitives.
func TestPreventionRiderResolvedPredicate(t *testing.T) {
	t.Parallel()
	svars := map[string]string{
		"Rider":  "DB$ PutCounter | Defined$ ShieldEffectTarget | CounterType$ P1P1 | CounterNum$ PreventedDamage",
		"Dealt":  "DB$ DealDamage | Defined$ ShieldEffectTarget | NumDmg$ PreventedDamage",
		"Unread": "DB$ Effect | RememberObjects$ ShieldEffectTarget",
		"Prev1":  "DB$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Rider | ShieldEffectTarget$ Targeted",
		"Prev2":  "DB$ PreventDamage | ValidTgts$ Any | Amount$ 3 | PreventionSubAbility$ Dealt | ShieldEffectTarget$ You",
		"Prev3":  "DB$ PreventDamage | ValidTgts$ Creature | Amount$ 2",
		"Prev4":  "DB$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Unread | ShieldEffectTarget$ Targeted",
		"Prev5":  "DB$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Rider | ShieldEffectTarget$ Enchanted",
		"Prev6":  "DB$ PreventDamage | ValidTgts$ Creature | Amount$ 3 | PreventionSubAbility$ Nowhere | ShieldEffectTarget$ Targeted",
	}
	resolved := []*SA{ResolveSVar(svars, "Prev1"), ResolveSVar(svars, "Prev2"), ResolveSVar(svars, "Prev3")}
	unresolved := []*SA{ResolveSVar(svars, "Prev4"), ResolveSVar(svars, "Prev5"), ResolveSVar(svars, "Prev6")}
	for i, sa := range resolved {
		if sa == nil {
			t.Fatalf("resolved SVar Prev%d did not parse", i+1)
		}
		if !PreventionRiderResolved(svars, sa) {
			t.Fatalf("PreventionRiderResolved = false, want true, for %v", sa.Params)
		}
	}
	for i, sa := range unresolved {
		if sa == nil {
			t.Fatalf("unresolved SVar Prev%d did not parse", i+4)
		}
		if PreventionRiderResolved(svars, sa) {
			t.Fatalf("PreventionRiderResolved = true, want false, for %v", sa.Params)
		}
	}
	// Precondition: the two groups really differ, so the test cannot pass on
	// a constant predicate.
	if reflect.DeepEqual(resolved, unresolved) {
		t.Fatal("vacuous: the resolved and unresolved groups are identical")
	}
}
