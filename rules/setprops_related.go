package rules

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/state"
)

// TargetsWithRelatedProperty$ (Spawnbroker LEPower, Puca's Mischief LECMC) is a
// reference-relative candidate filter: the candidate's power / mana value may
// not exceed the ParentTarget's (the parent ability's chosen target). The
// census post-filter and the CR 608.2b recheck both go through
// relatedPropertyAdmits, so offer and recheck cannot disagree.

type relatedPropertyCode uint16

const (
	relatedPropertyLECMC relatedPropertyCode = iota + 1
	relatedPropertyLEPower
)

var relatedPropertyCodes = state.NewStrCodes(
	state.StrEntry[relatedPropertyCode]{Key: "LECMC", Val: relatedPropertyLECMC},
	state.StrEntry[relatedPropertyCode]{Key: "LEPower", Val: relatedPropertyLEPower},
)

// relatedPropertyAdmits reports whether obj's property is <= ref's. A zero or
// vanished reference, a faceless object, or an unknown property fails closed.
func (e *Engine) relatedPropertyAdmits(prop string, obj, ref state.ObjID) bool {
	if obj == 0 || ref == 0 || obj == ref {
		return false
	}
	o, r := e.G.Obj(obj), e.G.Obj(ref)
	if o == nil || r == nil {
		return false
	}
	switch relatedPropertyCodes.Code(prop) {
	case relatedPropertyLECMC:
		if o.Face() == nil || r.Face() == nil {
			return false
		}
		return o.Face().Cmc() <= r.Face().Cmc()
	case relatedPropertyLEPower:
		return e.Power(obj) <= e.Power(ref)
	}
	return false
}

// relatedPropertyRecheck is the resolution-time view of the key: the property
// and the reference resolved ONCE before the per-target loop. The zero value
// (key absent) admits everything.
type relatedPropertyRecheck struct {
	prop string
	ref  state.ObjID
}

func (e *Engine) newRelatedPropertyRecheck(sa *cards.SA, source state.ObjID, sc effects.SpecContext) relatedPropertyRecheck {
	if sa == nil {
		return relatedPropertyRecheck{}
	}
	prop := effects.TargetsOf(sa).RelatedProperty
	if prop == "" {
		return relatedPropertyRecheck{}
	}
	return relatedPropertyRecheck{prop: prop, ref: e.sharedCardTypeReference("ParentTarget", source, sc)}
}

func (r relatedPropertyRecheck) admits(e *Engine, obj state.ObjID) bool {
	return r.prop == "" || e.relatedPropertyAdmits(r.prop, obj, r.ref)
}

// filterTargetsWithRelatedProperty drops the candidates whose property exceeds
// the ParentTarget's. Key present with an unresolved reference drops all.
func (e *Engine) filterTargetsWithRelatedProperty(in []targetCandidate, sa *cards.SA, source state.ObjID, sc effects.SpecContext) []targetCandidate {
	if sa == nil {
		return in
	}
	chk := e.newRelatedPropertyRecheck(sa, source, sc)
	if chk.prop == "" {
		return in
	}
	out := in[:0]
	for _, c := range in {
		if c.kind == "player" {
			continue
		}
		if chk.admits(e, c.obj) {
			out = append(out, c)
		}
	}
	return out
}
