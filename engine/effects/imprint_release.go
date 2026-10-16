package effects

import "github.com/adams-shaun/gorge/engine/state"

// ImprintedEffectReleaser is the optional host capability ClearImprinted$
// uses to make the host forget the effects an ImprintOnHost$ True Effect
// imprinted on it (rules.Engine.ReleaseImprintedEffects). Forge's Cleanup
// clears the host's imprinted list, which holds the created effect TOKEN; a
// later `Defined$ Imprinted | Origin$ Command` exile on that host then no
// longer ends the old effect. A host without the capability (the effects
// test double, fuzz hosts) keeps the marker, its pre-existing behaviour.
type ImprintedEffectReleaser interface {
	ReleaseImprintedEffects(source state.ObjID)
}

func releaseImprintedEffects(h Host, source state.ObjID) {
	if r, ok := h.(ImprintedEffectReleaser); ok {
		r.ReleaseImprintedEffects(source)
	}
}
