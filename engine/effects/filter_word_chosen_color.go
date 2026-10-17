package effects

import (
	"strings"

	"github.com/adams-shaun/gorge/engine/state"
)

func chosenColorMatches(g *state.Game, o *state.Object, sc SpecContext) bool {
	if sc.Source == 0 {
		return false
	}
	_, col := sourceChosen(g, sc)
	chosen := colourLetter(col)
	return chosen != 0 && strings.Contains(colorsCtx(o, &sc), string(chosen))
}
