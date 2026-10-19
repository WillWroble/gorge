package rules

import (
	"github.com/adams-shaun/gorge/engine/rules/chars"
	"github.com/adams-shaun/gorge/engine/state"
)

// ViewCharacteristics is Name(id), Keywords(id), Power(id) and Toughness(id)
// in one call -- the four derived facts a view.CardView projects -- read off
// one Derived instead of two Derived and two derivedScalar walks. Derived's
// P/T is the same layer-7 walk derivedScalar runs: derivedScalar only skips
// binding the finished keyword list when no layer-7 effect's Affects$ reads
// keywords (and otherwise delegates to Derived itself), and its type list and
// 7a basis are the ones derivedCompute passes. Keywords aliases Engine
// scratch storage exactly as Derived does: copy it before the next
// characteristics query.
//
// nameSet is effects.Chars.NameSet: the name is BOUND, so the empty string is
// a real name (a both-locked Room, CR 709.5) and the caller must not fall
// back to the printed face. The printed fast path answers only printed names,
// so it reports false.
func (e *Engine) ViewCharacteristics(id state.ObjID) (name string, nameSet bool, keywords []string, power, toughness int32) {
	if name, keywords, power, toughness, ok := e.printedViewCharacteristics(id); ok {
		return name, false, keywords, power, toughness
	}
	d := e.Derived(id)
	return d.Name, d.NameSet, d.Keywords, d.Power, d.Toughness
}

// ViewTypes is the view's derived type line: ok is true, with the layer-4
// type list, when a type-changing effect left id with a different set of
// type words than its face prints -- Imprisoned in the Moon's "is a
// colorless land ... and loses all other card types" on a creature, an
// animated land, Infernal Vessel's returned Demon. It reads the layer-4
// table the filter tier already binds (layer4types.go: refreshed after every
// event, an entry only for an object whose derived list differs from its
// face), so an unchanged board costs one empty-slice scan. An
// all-creature-types grant changes the line only with a card-type change: a
// printed changeling keeps its printed spelling. The returned slice is the
// table's own: use it before the next event.
func (e *Engine) ViewTypes(id state.ObjID) (types []string, ok bool) {
	for i := range e.layer4Types {
		ot := &e.layer4Types[i]
		if ot.ID != id {
			continue
		}
		o := e.G.Obj(id)
		if o == nil || o.Face() == nil {
			return nil, false
		}
		if ot.AllCreatureTypes && sameCardTypes(ot.Types, o.Face().Types) {
			return nil, false
		}
		return ot.Types, true
	}
	return nil, false
}

// sameCardTypes reports whether a and b name the same set of card types.
func sameCardTypes(a, b []string) bool {
	return cardTypesIn(a, b) && cardTypesIn(b, a)
}

func cardTypesIn(a, b []string) bool {
	for _, t := range a {
		if !chars.IsCardType(t) {
			continue
		}
		found := false
		for _, u := range b {
			if u == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
