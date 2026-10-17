package effects

import "github.com/adams-shaun/gorge/engine/state"

// sourceChosen is the ONE read of the spec source's recorded "as this enters"
// choices (ChosenType, ChosenColor) for the filter predicates ChosenType,
// IsNotChosenType, cmcChosenEvenOdd and ChosenColor. A source that has left
// the battlefield since its ability was created has had them cleared
// (engine/events/apply_zone.go), so rules binds the last-known values on the
// SpecContext (CR 113.7a/608.2h) and they win; otherwise the live source is
// read. Empty strings mean no choice, or no source.
func sourceChosen(g *state.Game, sc SpecContext) (chosenType, chosenColor string) {
	if sc.SourceLKIValid {
		return sc.SourceChosenType, sc.SourceChosenColor
	}
	if s := g.Obj(sc.Source); s != nil {
		return s.ChosenType, s.ChosenColor
	}
	return "", ""
}
