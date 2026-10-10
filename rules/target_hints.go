package rules

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/state"
)

// EnsureTargetHints lazily fills decision.Option.TargetHints on the pending
// priority decision: for each plain cast or printed activated ability whose
// root SpellAbility is a targeted HOSTILE effect (direct damage, or destroy,
// exile, graveyard, sacrifice), the number of legal targets under the deciding
// seat's control (a player target being that seat) and under opponents. It is
// the EnsurePaymentActions pattern: a pure derived read, no event, no state
// write, opt-in per seat, built once per decision. Options with no target step
// cost one switch; the engine never filters on the result. The counts are the
// pure target census the offer and the ask share (legalTargetCandidates), so
// they are the seats of the options the follow-up KTarget decision will pose.
func (e *Engine) EnsureTargetHints() {
	d := e.pending
	if d == nil || d.Kind != decision.KPriority || e.targetHintsSeq == d.Seq+1 {
		return
	}
	e.targetHintsSeq = d.Seq + 1
	p := d.Acting()
	e.BeginDerivedReads()
	for i := range d.Options {
		opt := &d.Options[i]
		var sa *cards.SA
		var excludeSelf state.ObjID
		switch {
		case opt.Kind == "cast" && opt.Mode == "" && opt.AltCostIndex == 0:
			o := e.G.Obj(opt.Obj)
			if o == nil || o.Face() == nil {
				continue
			}
			sa = o.Face().SpellAbility()
			excludeSelf = opt.Obj
		case opt.Kind == "ability" && opt.Mode == "" && opt.Keyword == "" && opt.SVar == "" &&
			opt.GainedSource == 0 && opt.GrantSource == 0 && opt.Ability >= 0:
			o := e.G.Obj(opt.Obj)
			if o == nil || o.Face() == nil {
				continue
			}
			if pa, ok := o.PileAbilityAt(opt.Ability); ok {
				sa = pa.SA
			}
		default:
			continue
		}
		if sa == nil || !hostileTargetSA(sa) || !effects.TargetsOf(sa).Targeted() {
			continue
		}
		own, opp := 0, 0
		for _, c := range e.legalTargetCandidates(p, opt.Obj, excludeSelf, sa) {
			if c.player == p {
				own++
			} else {
				opp++
			}
		}
		opt.TargetHints = decision.MakeTargetHints(own, opp, true)
	}
	e.EndDerivedReads()
}

// hostileTargetSA is the hint's hostility test, the same classification the
// KTarget decision's TargetEffect publishes: direct damage, or removal of kind
// destroy, exile, graveyard or sacrifice.
func hostileTargetSA(sa *cards.SA) bool {
	switch sa.APIKind() {
	case cards.APIDealDamage, cards.APIDamageAll:
		return true
	}
	if r := targetRemoval(sa); r != nil {
		switch r.Kind {
		case "destroy", "exile", "graveyard", "sacrifice":
			return true
		}
	}
	return false
}
