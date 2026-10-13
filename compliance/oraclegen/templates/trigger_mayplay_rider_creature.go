package templates

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/compliance/levelb"
	"github.com/adams-shaun/gorge/compliance/oraclegen"
)

// mayPlayRiderCreatureProbes are creature spells the may-play rider's cause
// casts when the permission static's own offered scenario plays a land
// instead (Mikey & Don, Party Planners: "You may play lands and cast Mutant,
// Ninja, or Turtle spells from the top of your library. If you cast a
// creature spell this way, that creature enters with an additional +1/+1
// counter on it" -- the static's served row offers the Forest it tries
// first, which observes the permission but casts nothing the rider can
// modify). Ninja of the Deep Hours ({1}{U}, Human Ninja) satisfies a
// Mutant/Ninja/Turtle filter; the static's own filter decides, through the
// offered scenario and its control, exactly as for the static's row.
var mayPlayRiderCreatureProbes = []string{"Ninja of the Deep Hours"}

// mayPlayRiderCreatureBase returns the offered scenario of a creature-spell
// probe for the rider's permission static, or false when none is offered
// (with the static) and refused (without it).
func mayPlayRiderCreatureBase(reg *cards.Registry, c *cards.Card, f *cards.Face, name string, sreq levelb.Requirement, st cards.Static) (oraclegen.Item, bool) {
	free := false
	var cast *oraclegen.Item
	if base, why := staticBase(reg, c, f, name, sreq, staticProbePlan{}, nil); why == "" {
		cast = &base
	}
	for _, z := range offerZones(st.ParamStr(cards.PKAffectedZone)) {
		for _, p := range mayPlayRiderCreatureProbes {
			pc, ok := reg.Lookup(p)
			if !ok || len(pc.Faces) == 0 || !pc.Faces[0].IsCreature() {
				continue
			}
			t, ok := offerTryFor(reg, p, z, free, false)
			if !ok || t.kind != offerKindCast {
				continue
			}
			for _, base := range []*oraclegen.Item{cast, nil} {
				if it, ok := offerItem(reg, f, name, sreq, t, base); ok {
					return it, true
				}
			}
		}
	}
	return oraclegen.Item{}, false
}
