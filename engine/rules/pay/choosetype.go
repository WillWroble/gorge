package pay

import (
	"github.com/adams-shaun/gorge/engine/effects"
	costvocab "github.com/adams-shaun/gorge/engine/rules/cost"
	"github.com/adams-shaun/gorge/engine/state"
)

// chosenTypeTerm is the filter term naming the source's chosen creature type.
const chosenTypeTerm = "ChosenType"

// BeholdChosenTypeOpen reports whether a Behold part's spec reads a creature
// type the SAME cost has yet to choose: the cost carries a
// ChooseCreatureType<N> part (Celestial Reunion's "choose a creature type and
// behold two creatures of that type") and the source has no chosen type
// recorded yet. Such a part is judged existentially at the offer gate
// (BeholdChosenTypeFeasible); once the type is chosen the ordinary matcher
// reads it off the source.
func BeholdChosenTypeOpen(e Engine, source state.ObjID, cost *costvocab.Cost, part costvocab.CostPart) bool {
	if cost.ChooseType <= 0 || !costvocab.SpecHasTerm(part.Spec, chosenTypeTerm) {
		return false
	}
	o := e.Game().Obj(source)
	return o == nil || o.ChosenType == ""
}

// BeholdChosenTypeFeasible reports whether SOME creature type leaves at
// least part.N beholdable objects -- cards in p's hand other than the source,
// and permanents p controls -- matching part's spec with that type in place
// of ChosenType. The types tried are the creature subtypes the candidates
// themselves print, in candidate order; a changeling matches each of them
// through the ordinary type predicate.
func BeholdChosenTypeFeasible(e Engine, p state.PlayerID, source state.ObjID, part costvocab.CostPart) bool {
	open := costvocab.ReplaceTerm(part.Spec, chosenTypeTerm, "")
	cands := append(CostCandidates(e, p, source, state.ZHand, open, true, false),
		CostCandidates(e, p, source, state.ZBattlefield, open, false, false)...)
	if len(cands) < int(part.N) {
		return false
	}
	var tried []string
	for _, id := range cands {
		o := e.Game().Obj(id)
		if o == nil || o.Face() == nil {
			continue
		}
		for _, t := range o.Face().Types {
			if !effects.CreatureTypeWords(t) || containsString(tried, t) {
				continue
			}
			tried = append(tried, t)
			if BeholdChosenTypeCount(e, p, source, part, t) >= int(part.N) {
				return true
			}
		}
	}
	return false
}

// BeholdChosenTypeCount counts the beholdable objects -- cards in p's hand
// other than the source, permanents p controls -- matching part's spec with
// the creature type t in place of ChosenType.
func BeholdChosenTypeCount(e Engine, p state.PlayerID, source state.ObjID, part costvocab.CostPart, t string) int {
	spec := costvocab.ReplaceTerm(part.Spec, chosenTypeTerm, t)
	return len(CostCandidates(e, p, source, state.ZHand, spec, true, false)) +
		len(CostCandidates(e, p, source, state.ZBattlefield, spec, false, false))
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
