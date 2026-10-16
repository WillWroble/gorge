package cards

import "strings"

// The prevention-shield rider shape (SP$/AB$/DB$ PreventDamage with a
// PreventionSubAbility$): the amount one application prevented feeds a single
// follow-up ability, whose targets the shield's ShieldEffectTarget$ scope
// names. These predicates are the ONE home every reader shares — the coverage
// walk's api:PreventDamage.OtherRider synthetic (primitive.go), the
// registering primitive's install-time scope gate (engine/effects/prevent_damage.go)
// and rules' per-application resolution gate (engine/rules/replacement_damage.go) —
// so a rider family implemented on one side cannot stay counted supported on
// the other.

// PreventionShieldTargetBound reports whether a PreventDamage ability's
// ShieldEffectTarget$ scope is one the registering primitive binds: the
// parent SA's chosen targets (ParentTarget — the enclosing Pump's redirect
// target), the shield's own recipients (Targeted — the prevented creature),
// or the shield's controller (You).
func PreventionShieldTargetBound(scope string) bool {
	switch strings.TrimSpace(scope) {
	case "ParentTarget", "Targeted", "You":
		return true
	}
	return false
}

// PreventionRiderAPI reports whether a PreventionSubAbility$ rider body's
// API is one rules resolves at prevention time with the prevented amount
// bound into its numeric parameter: DealDamage (NumDmg$ PreventedDamage),
// PutCounter (CounterNum$ PreventedDamage) and GainLife (LifeAmount$
// PreventedDamage).
func PreventionRiderAPI(api string) bool {
	switch strings.TrimSpace(api) {
	case "DealDamage", "PutCounter", "GainLife":
		return true
	}
	return false
}

// DelayedPreventedCounterBody reports whether the SVar named exec is the
// delayed counter body of a prevention rider: a PutCounter whose
// CounterNum$ is PreventedDamage (Sacred Boon's DelayedBoon, Scars of the
// Veteran's DelayedScars). The amount it names is read back at fire time from
// the registration's remembered number.
func DelayedPreventedCounterBody(svars map[string]string, exec string) bool {
	sub := ResolveSVar(svars, exec)
	return sub != nil && strings.TrimSpace(sub.API) == "PutCounter" &&
		strings.EqualFold(strings.TrimSpace(sub.ParamStr(PKCounterNum)), "PreventedDamage")
}

// PreventionRiderDelayedCounterEffect reports whether sub is the one DB$
// Effect rider shape the engine resolves (Sacred Boon, Scars of the
// Veteran): its single Triggers$ SVar is a Mode$ Phase trigger whose
// Execute$ is a DelayedPreventedCounterBody. Any other Effect rider stays
// unresolved.
func PreventionRiderDelayedCounterEffect(svars map[string]string, sub *SA) bool {
	if sub == nil || strings.TrimSpace(sub.API) != "Effect" {
		return false
	}
	names := strings.Fields(sub.ParamStr(PKTriggers))
	if len(names) != 1 {
		return false
	}
	tr, ok := ParseTriggerLine(svars[names[0]])
	if !ok || strings.TrimSpace(tr.Mode) != "Phase" {
		return false
	}
	return DelayedPreventedCounterBody(svars, strings.TrimSpace(tr.ParamStr(PKExecute)))
}

// PreventionRiderResolved reports whether sa (a PreventDamage ability)
// carries a PreventionSubAbility$ rider this engine resolves: the
// ShieldEffectTarget$ scope must be bound and the named SVar must resolve to
// an ability whose API is one of the resolved rider APIs. A rider-less
// shield is resolved by definition; a rider whose SVar does not resolve to
// an ability (a value expression) is not.
func PreventionRiderResolved(svars map[string]string, sa *SA) bool {
	name := strings.TrimSpace(sa.ParamStr(PKPreventionSubAbility))
	if name == "" {
		return true
	}
	if !PreventionShieldTargetBound(sa.ParamStr(PKShieldEffectTarget)) {
		return false
	}
	sub := ResolveSVar(svars, name)
	return sub != nil && (PreventionRiderAPI(sub.API) || PreventionRiderDelayedCounterEffect(svars, sub))
}
