package rules

import (
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/effects"
	costvocab "github.com/adams-shaun/gorge/engine/rules/cost"
	"github.com/adams-shaun/gorge/engine/rules/pay"
	"github.com/adams-shaun/gorge/engine/state"
)

// cost_target_bound.go is the target-bound cost filter: a Sac or Discard
// cost part whose spec compares against a variable the ability's TARGETS fix
// -- Sidisi, Regent of the Mire's "Sacrifice a creature you control with mana
// value X other than Sidisi: Return target creature card with mana value X
// plus 1" (Sac<1/Creature.Other+cmcEQX>, SVar:X:Targeted$CardManaCost/
// Minus.1), Scrap Welder's Sac<1/Artifact.cmcGTX> and Kozilek, the Great
// Distortion's Discard<1/Card.cmcEQX> (SVar:X:Targeted$CardManaCost).
//
// CR 601.2c (via 602.2b) chooses targets before CR 601.2h pays costs, so the
// X is known only once the targets are. The engine settles every other
// non-mana cost part BEFORE the target stage; a target-bound part is
// therefore DEFERRED past it (sacAsk/discardAsk skip it until the flow has
// passed the target stage, and postTargetAsks poses it), and its spec is
// matched with the X evaluated against the chosen targets
// (payer.MatchesSpecFrom). Before targets exist the comparison is judged
// open (StripVarCompares) -- the offer gate's reading -- and the target ask
// withholds a target whose X leaves the part unpayable, so neither an
// offered activation nor a chosen target can strand the payment.

// costTargetVarName is the one variable name a target-bound cost spec is
// read for: every corpus carrier spells it X (TestTargetBoundCostCensus).
const costTargetVarName = "X"

// svarReadsTargets reports whether an SVar body is evaluated against the
// resolving ability's targets (Targeted$CardManaCost and its arithmetic).
func svarReadsTargets(body string) bool {
	return strings.HasPrefix(body, "Targeted$")
}

// costSpecTargetBound reports whether spec, read for source's cost, compares
// against an X the source's SVar table binds to the targets. The first test
// is one substring scan, so the ordinary cost spec pays nothing more.
func (e *Engine) costSpecTargetBound(spec string, source state.ObjID) bool {
	if !costvocab.SpecComparesVar(spec, costTargetVarName) {
		return false
	}
	o := e.G.Obj(source)
	if o == nil || o.Face() == nil {
		return false
	}
	return svarReadsTargets(o.Face().SVars[costTargetVarName])
}

// targetBoundSpecContext is source's spec context with the X resolved
// against targets: the SVar body evaluated in a context whose targets are
// exactly the given ones.
func (e *Engine) targetBoundSpecContext(source state.ObjID, you state.PlayerID, targets []state.Target) effects.SpecContext {
	sc := e.specCtx(source, you)
	prev := sc.Resolve
	var svars map[string]string
	if o := e.G.Obj(source); o != nil && o.Face() != nil {
		svars = o.Face().SVars
	}
	sc.Resolve = func(name string) (int32, bool) {
		if name != costTargetVarName {
			return prev(name)
		}
		body, ok := svars[name]
		if !ok {
			return 0, false
		}
		ctx := effects.NewCtxPtr(source, you, effects.CtxInit{SVars: svars, Targets: targets})
		return effects.EvalCountOK(e, ctx, body)
	}
	return sc
}

// matchTargetBoundCostSpec is payer.MatchesSpecFrom for a target-bound spec:
// once the pending activation of source has passed its target stage, the X
// is the chosen targets' value; before that (the offer gate, the cost walks
// that run before targets) the comparison is open.
func (e *Engine) matchTargetBoundCostSpec(spec string, id state.ObjID, you state.PlayerID, source state.ObjID) bool {
	if pc := e.cast; pc != nil && pc.card == source && pc.passedTarget {
		return e.matchesSpec(spec, id, e.targetBoundSpecContext(source, you, pc.targets))
	}
	return e.matchesSpecFrom(costvocab.StripVarCompares(spec, costTargetVarName), id, you, source)
}

// costPartDeferred reports whether a Sac/Discard part of the pending cast is
// target-bound and the flow has not yet passed the target stage: the part is
// paid after targets (postTargetAsks).
func (e *Engine) costPartDeferred(pc *pendingCast, spec string) bool {
	return !pc.passedTarget && e.costSpecTargetBound(spec, pc.card)
}

// targetBoundCostPending reports whether the pending cast still owes a
// target-bound Sac or Discard part.
func (e *Engine) targetBoundCostPending(pc *pendingCast) bool {
	for i := pc.SacPart; i < len(pc.cost.Sac); i++ {
		if e.costSpecTargetBound(pc.cost.Sac[i].Spec, pc.card) {
			return true
		}
	}
	for i := pc.DiscardPart; i < len(pc.cost.Discard); i++ {
		if e.costSpecTargetBound(pc.cost.Discard[i].Spec, pc.card) {
			return true
		}
	}
	return false
}

// targetBoundCostAsks poses the deferred target-bound parts once targets
// exist: the ordinary sacrifice and discard asks, which now match with the
// bound X. It is a postTargetAsks stage.
func (e *Engine) targetBoundCostAsks(pc *pendingCast) bool {
	if !e.targetBoundCostPending(pc) {
		return false
	}
	return e.sacAsk() || e.discardAsk()
}

// targetBoundPartsPayable reports whether every target-bound Sac/Discard
// part of cost has enough candidates with X bound to targets. Parts that are
// not target-bound are not judged (the ordinary offer and payment gates
// own them).
func (e *Engine) targetBoundPartsPayable(p state.PlayerID, source state.ObjID, cost *Cost, ability bool, targets []state.Target) bool {
	sc := e.targetBoundSpecContext(source, p, targets)
	cause := pay.CostCauseForAbility(ability)
	used := map[state.ObjID]bool{}
	for _, part := range cost.Sac {
		spec := pay.SacrificeMatchSpec(part.Spec)
		if !e.costSpecTargetBound(spec, source) {
			continue
		}
		n := int32(0)
		for _, oid := range e.G.Zone(state.ZBattlefield, p) {
			if used[oid] || e.sacrificeBlockedForCost(oid, cause) || !e.matchesSpec(spec, oid, sc) {
				continue
			}
			if n < part.N {
				used[oid] = true
			}
			n++
		}
		if n < part.N {
			return false
		}
	}
	for _, part := range cost.Discard {
		if !e.costSpecTargetBound(part.Spec, source) {
			continue
		}
		n := int32(0)
		for _, oid := range e.G.Zone(state.ZHand, p) {
			if used[oid] || (!ability && oid == source) || !e.matchesSpec(part.Spec, oid, sc) {
				continue
			}
			if n < part.N {
				used[oid] = true
			}
			n++
		}
		if n < part.N {
			return false
		}
	}
	return true
}

// costHasTargetBoundPart reports whether cost carries a target-bound Sac or
// Discard part for source.
func (e *Engine) costHasTargetBoundPart(cost *Cost, source state.ObjID) bool {
	for _, part := range cost.Sac {
		if e.costSpecTargetBound(part.Spec, source) {
			return true
		}
	}
	for _, part := range cost.Discard {
		if e.costSpecTargetBound(part.Spec, source) {
			return true
		}
	}
	return false
}

// candidateTarget is a target census entry as the state.Target a chosen
// answer would record.
func candidateTarget(c targetCandidate) state.Target {
	if c.kind == "player" {
		return state.Target{Player: c.player, IsPlayer: true}
	}
	return state.Target{Obj: c.obj}
}

// targetBoundCandidates withholds the targets whose bound X leaves a
// target-bound cost part unpayable (the target ask's twin of
// affordableTargetCandidates: choosing such a target could only end in the
// CR 733.1 reversal). Each candidate is judged as the sole target, which is
// every carrier's shape.
func (e *Engine) targetBoundCandidates(p state.PlayerID, source state.ObjID, cost *Cost, ability bool, candidates []targetCandidate) []targetCandidate {
	if !e.costHasTargetBoundPart(cost, source) {
		return candidates
	}
	out := make([]targetCandidate, 0, len(candidates))
	for _, c := range candidates {
		if e.targetBoundPartsPayable(p, source, cost, ability, []state.Target{candidateTarget(c)}) {
			out = append(out, c)
		}
	}
	return out
}

// abilityTargetBoundAvailable is the activated-ability offer guard's
// target-bound arm: an ability whose cost carries a target-bound part is
// offered only when some legal target leaves that part payable.
func (e *Engine) abilityTargetBoundAvailable(p state.PlayerID, id, excludeSelf state.ObjID, ab *cards.SA) bool {
	raw := ab.ParamStr(cards.PKCost)
	if !strings.Contains(raw, costTargetVarName) {
		return true
	}
	cost := e.parseCost(raw)
	if !e.costHasTargetBoundPart(&cost, id) {
		return true
	}
	return len(e.targetBoundCandidates(p, id, &cost, true, e.legalTargetCandidates(p, id, excludeSelf, ab))) > 0
}
