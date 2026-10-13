package pay

import (
	"slices"
	"strings"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/state"
)

// PaymentPlanReflectedShape reports whether a reflected-mana ability (AB$
// ManaReflected) has the one shape V1 can plan: Exotic Orchard's "add one
// mana of any color that a land an opponent controls could produce". Its
// colours are a finite choice the census resolves with the resolution's own
// candidate walk (effects.ManaReflectedCandidates), and that choice cannot
// move while the payment runs: ReflectProperty$ Produce reads the reflected
// objects' printed mana abilities (never their tapped state), and every
// Valid$ alternative names an object an OPPONENT controls, which no step of
// the payer's own payment taps, sacrifices or returns. Everything else stays
// deferred: an "Is" (colour) read, a Defined.* selector (Chrome Mox's
// imprint, Squandered Resources' sacrificed land), a reflection of the
// payer's own permanents (a planned self-sacrifice could remove one), an
// Amount$, a RestrictValid$, an activation gate, or a Produced$ override.
func PaymentPlanReflectedShape(ma *cards.SA) bool {
	if ma == nil || ma.APIKind() != cards.APIManaReflected {
		return false
	}
	mr := effects.ManaReflectedOf(ma)
	if !paymentPlanReflectedProperties.Has(mr.ReflectProperty) || mr.Produced != "" || mr.Amount.Present ||
		mr.RestrictValid != "" || mr.ClassBand != "" || mr.IsPresent != "" {
		return false
	}
	return reflectsOpponentsOnly(mr.Valid)
}

// reflectsOpponentsOnly reports whether every comma-separated alternative of
// a Valid$ object spec carries the OppCtrl property.
func reflectsOpponentsOnly(spec string) bool {
	if spec == "" || strings.HasPrefix(spec, "Defined.") {
		return false
	}
	for alt := range strings.SplitSeq(spec, ",") {
		_, props, ok := strings.Cut(strings.TrimSpace(alt), ".")
		if !ok || !slices.Contains(strings.Split(props, "+"), "OppCtrl") {
			return false
		}
	}
	return true
}

// PaymentPlanReflectedCounts is the census's per-colour production of a
// PaymentPlanReflectedShape ability: one count per colour cols names (the
// resolution's candidate list), in the WindowAlt Counts convention a
// Produced$ Any alternative already uses. ok is false when no colour is
// named.
func PaymentPlanReflectedCounts(cols []string) (counts [6]int32, ok bool) {
	counts, _ = cards.ProducedCounts(strings.Join(cols, " "))
	for _, n := range counts {
		if n > 0 {
			return counts, true
		}
	}
	return counts, false
}

// planAltChoiceColours is the colour list a choice-shaped census alternative
// expands into: a reflected ability's own census counts (the candidates the
// census resolved), else PaymentPlanChoiceColours' read of Produced$.
func planAltChoiceColours(e Engine, id state.ObjID, alt WindowAlt) []string {
	if alt.Ma.APIKind() != cards.APIManaReflected {
		return PaymentPlanChoiceColours(e, id, alt.Ma)
	}
	if !PaymentPlanReflectedShape(alt.Ma) {
		return nil
	}
	var cols []string
	for i, n := range alt.Counts {
		if n > 0 {
			cols = append(cols, string(cards.ManaSymbol(i)))
		}
	}
	return cols
}

// paymentPlanReflectedProperties is the ReflectProperty$ set
// PaymentPlanReflectedShape admits.
var paymentPlanReflectedProperties = state.NewNameSet("Produce")

// paymentPlanReflectedParamSet is the reflected-only parameters the shape
// gate's key walk accepts on a PaymentPlanReflectedShape ability (their
// values are vetted by that predicate).
var paymentPlanReflectedParamSet = state.NewNameSet("ColorOrType", "ReflectProperty", "Valid")
