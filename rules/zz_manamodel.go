//go:build manamodel

package rules

// Diagnostic-only helpers for the mana-model gap measurement
// (cmd/manamodelstats, docs/superpowers/reports/2026-10-10-mana-model-gap.md).
// This file is compiled ONLY with -tags manamodel, so the production engine,
// its tests and every golden are untouched.
//
// MMProbe is a pure read of the live engine: no event, no state write, no
// RNG. MMRealize works only on clones of the engine (Clone) and never touches
// the engine it is called on.
//
// Three source censuses are compared, each a list of per-source units whose
// alternatives are one tap's production (a source pays through at most one
// alternative):
//
//   - W, the shared payment-window census windowManaUnits (free-cost
//     abilities with a deterministic production): what AvailableMana, the
//     attack-tax budget and the unless-cost windows count;
//   - P, the auto-pay planner's census (paymentPlanManaUnits expanded by
//     PaymentPlanQueryAlternatives, normal and last-resort tiers): the only
//     sources an offered PaymentAction plan can spend;
//   - F, the FULL model: every mana ability of every mana source the seat can
//     reach (battlefield, plus hand/graveyard printed sources), whose tap part
//     is currently available, counted at its printed production with the
//     REST of its activation cost ignored. Choice-shaped productions are one
//     alternative per colour; amounts are the engine's own Amount$ evaluator
//     (castWindowAmount), and an amount it cannot price is left out.
//
// F is built as a superset of W and P (their alternatives are merged into
// it), so "payable under W" is exactly "payable from F's W-flagged
// alternatives".

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/rules/pay"
	"github.com/adams-shaun/gorge/state"
)

// MMClasses is the excluded-cost class order: an alternative outside a
// baseline census is attributed to the FIRST class its activation cost
// matches, and a withheld cast to the first class that alone makes it
// payable. FreeShape is a free-cost ability the census still excludes (a
// choice-shaped or evaluated production, an InstantSpeed$ or RestrictValid$
// ability, a tier the planner defers), split by the first matching reason:
// Free:Restricted (RestrictValid$), Free:InstantSpeed, Free:Amount (a
// non-literal Amount$), Free:Choice (a choice-shaped Produced$), Free:Tier
// (a plain production the census still withholds: the planner's tier gate,
// a rider, a trigger or replacement on the source).
var MMClasses = []string{"Sac", "Life", "ManaFee", "SubCounter", "TapOther", "Discard", "Exile", "AddCounter", "OtherCost",
	"Free:Restricted", "Free:InstantSpeed", "Free:Amount", "Free:Choice", "Free:Tier"}

// MMAlt is one production alternative of one source.
type MMAlt struct {
	Mana  state.Mana
	InW   bool
	InP   bool
	Class string // the activation cost's class (MMClasses)
	Token bool   // the source is a token
	// Detail, for an alternative outside P, is why the planner census
	// withholds it: the tier gate's detail string when it defers the
	// ability, else "census:<reason>" naming the census layer that drops a
	// tier-admitted ability.
	Detail string
	ma     *cards.SA
}

// MMUnit is one source: at most one of its alternatives is used.
type MMUnit struct {
	ID   state.ObjID
	Alts []MMAlt
}

// MMCandidate is one ordinary cast the seat could make if mana were no
// object, with its verdict under each census.
type MMCandidate struct {
	Obj  state.ObjID
	Name string
	// PayW/PayP/PayF: payable from the floating pool plus W/P/F sources.
	PayW, PayP, PayF bool
	// Known is false when a search exhausted its node budget.
	Known bool
	// ClassW/ClassP attribute a W/P-withheld, F-payable cast: the first
	// class (MMClasses, with ":token" when every alternative of that class
	// on the board comes from a token, ":mixed" when some do) whose
	// alternatives ALONE, added to the baseline, pay it; "combo" when no
	// single class does.
	ClassW, ClassP string
}

// MMProbeResult is one priority window's census comparison. ExcludedW
// (ExcludedP) reports whether F has any alternative outside W (P); when
// neither holds no cast can be withheld and the hand is not walked.
type MMProbeResult struct {
	ExcludedW, ExcludedP bool
	Candidates           []MMCandidate
	Units                []MMUnit
	Unpriced             int
}

const mmNodeBudget = 200000

func mmCostClass(c Cost) string {
	switch {
	case len(c.Sac) > 0:
		return "Sac"
	case c.Life > 0 || len(c.LifeX) > 0:
		return "Life"
	case c.Generic > 0 || c.Colored != (state.Mana{}) || c.X > 0 || len(c.Hybrid) > 0 || len(c.Phyrexian) > 0:
		return "ManaFee"
	case len(c.SubCounter) > 0:
		return "SubCounter"
	case len(c.TapPermanent) > 0:
		return "TapOther"
	case len(c.Discard) > 0:
		return "Discard"
	case len(c.Exile) > 0:
		return "Exile"
	case len(c.AddCounter) > 0:
		return "AddCounter"
	case !pay.ManaFreeCost(c):
		return "OtherCost"
	}
	return "FreeShape"
}

func mmAddAlt(units []MMUnit, id state.ObjID, a MMAlt) []MMUnit {
	idx := -1
	for i := range units {
		if units[i].ID == id {
			idx = i
			break
		}
	}
	if idx >= 0 {
		for k := range units[idx].Alts {
			x := &units[idx].Alts[k]
			if x.Mana == a.Mana && (x.ma == a.ma || (x.ma != nil && a.ma != nil && pay.SameManaAbility(x.ma, a.ma))) {
				x.InW = x.InW || a.InW
				x.InP = x.InP || a.InP
				return units
			}
		}
	}
	if idx < 0 {
		units = append(units, MMUnit{ID: id})
		idx = len(units) - 1
	}
	units[idx].Alts = append(units[idx].Alts, a)
	return units
}

// MMUnits builds the merged F census with W/P membership flags.
func (e *Engine) MMUnits(p state.PlayerID) (units []MMUnit, unpriced int) {
	e.beginDerivedMemo()
	defer e.endDerivedMemo()
	for _, id := range pay.ManaSourceIDs(e.G, p) {
		o := e.G.Obj(id)
		if o == nil || o.Face() == nil {
			continue
		}
		for _, ma := range e.appendAvailableManaAbilitiesGate(nil, nil, p, id, true) {
			cost := e.parseCost(ma.ParamStr(cards.PKCost))
			if pay.ActivationTapCostUnavailable(o, &cost) {
				continue
			}
			amt, ok := e.castWindowAmount(p, id, o, ma)
			if !ok {
				unpriced++
				continue
			}
			produced := effects.ManaOf(ma).Produced
			if produced == "" {
				produced = "C"
			}
			if producedNeedsChosen(produced) {
				chosen := pay.ChosenProducedColour(e.G, id)
				if chosen == "" {
					unpriced++
					continue
				}
				produced = pay.SubstituteChosenProduced(produced, chosen)
			}
			counts, any := cards.ProducedCounts(produced)
			class := mmCostClass(cost)
			if class == "FreeShape" {
				mp := effects.ManaOf(ma)
				switch {
				case mp.RestrictValid != "":
					class = "Free:Restricted"
				case pay.InstantSpeedOnly(ma):
					class = "Free:InstantSpeed"
				case !mp.AmountIsLit && mp.AmountTrim != "":
					class = "Free:Amount"
				case any:
					class = "Free:Choice"
				default:
					class = "Free:Tier"
				}
			}
			if any {
				for colour, n := range counts {
					if n == 0 {
						continue
					}
					var single [6]int32
					single[colour] = 1
					m := windowManaAlt{Counts: single, Amt: amt}.Mana()
					units = mmAddAlt(units, id, MMAlt{Mana: m, Class: class, Token: o.IsToken, ma: ma})
				}
				continue
			}
			m := windowManaAlt{Counts: counts, Amt: amt}.Mana()
			if m.Total() <= 0 {
				unpriced++
				continue
			}
			units = mmAddAlt(units, id, MMAlt{Mana: m, Class: class, Token: o.IsToken, ma: ma})
		}
	}
	for _, u := range e.windowManaUnits(p) {
		tok := false
		if o := e.G.Obj(u.ID); o != nil {
			tok = o.IsToken
		}
		for _, a := range u.Alts {
			units = mmAddAlt(units, u.ID, MMAlt{Mana: a.Mana(), InW: true, Class: "Free:Tier", Token: tok, ma: a.Ma})
		}
	}
	for _, u := range e.paymentPlanManaUnits(p) {
		tok := false
		if o := e.G.Obj(u.ID); o != nil {
			tok = o.IsToken
		}
		for _, a := range pay.PaymentPlanQueryAlternatives(asPayer(e), u) {
			units = mmAddAlt(units, u.ID, MMAlt{Mana: a.Mana, InP: true, Class: "Free:Tier", Token: tok, ma: a.Ma})
		}
	}
	for i := range units {
		for k := range units[i].Alts {
			if a := &units[i].Alts[k]; !a.InP {
				a.Detail = e.mmPDetail(p, units[i].ID, a)
			}
		}
	}
	return units, unpriced
}

// mmPDetail names why a is outside the planner census (MMAlt.Detail).
func (e *Engine) mmPDetail(p state.PlayerID, id state.ObjID, a *MMAlt) string {
	if a.ma == nil {
		return "census:no_ability"
	}
	if tier, _, d := pay.PaymentPlanAbilityTier(asPayer(e), p, id, a.ma); tier == pay.TierDeferred {
		if d == "" {
			d = "deferred"
		}
		return d
	}
	if o := e.G.Obj(id); o != nil && o.Tapped {
		return "census:tapped"
	}
	for _, u := range e.paymentPlanManaUnits(p) {
		if u.ID == id {
			return "census:production_differs"
		}
	}
	return "census:no_unit"
}

// mmCostCap bounds how much mana c can absorb, the per-colour cap of the
// search's failure memo.
func mmCostCap(c Cost) int32 {
	return c.Generic + c.Colored.Total() + int32(len(c.Hybrid)) + int32(len(c.Phyrexian)) +
		2*int32(len(c.Twobrid)) + int32(len(c.HybridPhyrexian)) + c.Snow
}

// mmPayable: is c payable from pool plus at most one allowed alternative per
// unit? known is false when the node budget ran out.
func (e *Engine) mmPayable(p state.PlayerID, c Cost, pool state.Mana, units []MMUnit, allow func(*MMAlt) bool, budget *int) (ok, known bool) {
	return e.mmPayableW(p, c, pool, units, allow, budget, nil)
}

// mmPayableW is mmPayable recording, on success, the witness: the units
// index of every source the found payment uses.
func (e *Engine) mmPayableW(p state.PlayerID, c Cost, pool state.Mana, units []MMUnit, allow func(*MMAlt) bool, budget *int, witness *[]int) (ok, known bool) {
	life := e.G.Players[p].Life
	payable := func(have state.Mana) bool {
		_, ok := resolveManaWith(c, have, state.Mana{}, [7]state.Mana{}, life, false, pipRider{}, nil)
		return ok
	}
	if payable(pool) {
		return true, true
	}
	// Per-unit allowed alternatives, and the suffix of each unit's max.
	type ua struct {
		alts []state.Mana
		max  int32
		unit int
	}
	var us []ua
	for i := range units {
		u := ua{unit: i}
		for k := range units[i].Alts {
			a := &units[i].Alts[k]
			if !allow(a) {
				continue
			}
			u.alts = append(u.alts, a.Mana)
			if t := a.Mana.Total(); t > u.max {
				u.max = t
			}
		}
		if len(u.alts) > 0 {
			us = append(us, u)
		}
	}
	need := c.Generic + c.Colored.Total() + int32(len(c.Hybrid)) + int32(len(c.Twobrid)) + int32(len(c.HybridPhyrexian))
	capN := mmCostCap(c)
	if capN < 1 {
		capN = 1
	}
	suffix := make([]int32, len(us)+1)
	for i := len(us) - 1; i >= 0; i-- {
		suffix[i] = suffix[i+1] + us[i].max
	}
	type key struct {
		at int
		m  state.Mana
	}
	failed := map[key]bool{}
	exhausted := false
	var walk func(at int, produced state.Mana) bool
	walk = func(at int, produced state.Mana) bool {
		if *budget <= 0 {
			exhausted = true
			return false
		}
		*budget--
		for i := range produced {
			if produced[i] > capN {
				produced[i] = capN
			}
		}
		k := key{at, produced}
		if failed[k] {
			return false
		}
		have := pay.ManaAdd(pool, produced)
		if have.Total() >= need && payable(have) {
			return true
		}
		if at == len(us) || have.Total()+suffix[at] < need {
			failed[k] = true
			return false
		}
		for _, m := range us[at].alts {
			if walk(at+1, pay.ManaAdd(produced, m)) {
				if witness != nil {
					*witness = append(*witness, us[at].unit)
				}
				return true
			}
		}
		if walk(at+1, produced) {
			return true
		}
		if !exhausted {
			failed[k] = true
		}
		return false
	}
	ok = walk(0, state.Mana{})
	return ok, ok || !exhausted
}

// mmCandidates lists the ordinary casts (no mode, no alternative cost) the
// offer walk would make against an unbounded pool: every timing, target,
// restriction and non-mana-cost gate is the engine's own.
func (e *Engine) mmCandidates(p state.PlayerID) []state.ObjID {
	huge := state.Mana{potentialUnbounded, potentialUnbounded, potentialUnbounded, potentialUnbounded, potentialUnbounded, potentialUnbounded}
	opts := e.legalActionsWalk(p, &huge, true)
	var out []state.ObjID
	for _, o := range opts {
		if o.Kind != optCast || o.Mode != "" || o.AltCostIndex != 0 || o.Obj == 0 {
			continue
		}
		dup := false
		for _, id := range out {
			if id == o.Obj {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, o.Obj)
		}
	}
	return out
}

// mmCost is the composed cost an ordinary cast of id is offered at.
func (e *Engine) mmCost(p state.PlayerID, id state.ObjID) Cost {
	return e.offerCostFor(p, id, pay.RawBaseCost(asPayer(e), p, id), spellScope(""))
}

func mmHasExcluded(units []MMUnit, inBase func(*MMAlt) bool) bool {
	for i := range units {
		for k := range units[i].Alts {
			if !inBase(&units[i].Alts[k]) {
				return true
			}
		}
	}
	return false
}

func mmInW(a *MMAlt) bool { return a.InW }
func mmInP(a *MMAlt) bool { return a.InP }

// mmAttribute names the first class whose excluded alternatives alone, added
// to the baseline, pay c.
func (e *Engine) mmAttribute(p state.PlayerID, c Cost, pool state.Mana, units []MMUnit, inBase func(*MMAlt) bool, budget *int) string {
	for _, class := range MMClasses {
		ok, _ := e.mmPayable(p, c, pool, units, func(a *MMAlt) bool {
			return inBase(a) || a.Class == class
		}, budget)
		if !ok {
			continue
		}
		tok, card := false, false
		for i := range units {
			for k := range units[i].Alts {
				a := &units[i].Alts[k]
				if !inBase(a) && a.Class == class {
					if a.Token {
						tok = true
					} else {
						card = true
					}
				}
			}
		}
		switch {
		case tok && !card:
			return class + ":token"
		case tok && card:
			return class + ":mixed"
		}
		return class + ":card"
	}
	return "combo"
}

// MMProbe compares the three censuses at p's current state. It is a pure
// read. When neither W nor P excludes any F alternative it returns without
// walking the hand (no cast can be withheld by the census then).
func (e *Engine) MMProbe(p state.PlayerID) MMProbeResult {
	units, unpriced := e.MMUnits(p)
	r := MMProbeResult{Units: units, Unpriced: unpriced}
	r.ExcludedW = mmHasExcluded(units, mmInW)
	r.ExcludedP = mmHasExcluded(units, mmInP)
	if !r.ExcludedW && !r.ExcludedP {
		return r
	}
	pool := e.G.Players[p].Pool
	for _, id := range e.mmCandidates(p) {
		c := e.mmCost(p, id)
		budget := mmNodeBudget
		cand := MMCandidate{Obj: id, Known: true}
		if o := e.G.Obj(id); o != nil && o.Face() != nil {
			cand.Name = o.Face().Name
		}
		var k1, k2, k3 bool
		cand.PayF, k1 = e.mmPayable(p, c, pool, units, func(*MMAlt) bool { return true }, &budget)
		if !cand.PayF {
			cand.Known = k1
			r.Candidates = append(r.Candidates, cand)
			continue
		}
		cand.PayW, k2 = e.mmPayable(p, c, pool, units, mmInW, &budget)
		cand.PayP, k3 = e.mmPayable(p, c, pool, units, mmInP, &budget)
		cand.Known = k1 && k2 && k3
		if !cand.PayW {
			cand.ClassW = e.mmAttribute(p, c, pool, units, mmInW, &budget)
		}
		if !cand.PayP {
			cand.ClassP = e.mmAttribute(p, c, pool, units, mmInP, &budget)
		}
		r.Candidates = append(r.Candidates, cand)
	}
	return r
}

// MMFullPayable is MMProbe's F verdict for one ordinary cast of obj.
func (e *Engine) MMFullPayable(p state.PlayerID, obj state.ObjID) (ok, known bool) {
	units, _ := e.MMUnits(p)
	budget := mmNodeBudget
	return e.mmPayable(p, e.mmCost(p, obj), e.G.Players[p].Pool, units, func(*MMAlt) bool { return true }, &budget)
}

// MMRealizeResult is the exact (L2) verdict for one cast.
type MMRealizeResult struct {
	Real bool // some sequence of real mana activations makes the engine offer the cast
	// Known is false when the search was cut (node budget or activation
	// depth) before either finding the cast or exhausting the space.
	Known bool
	// Guided: the F witness's own sources, activated in census order,
	// sufficed (no free search was needed).
	Guided bool
	Nodes  int
	// StackGrew: some explored activation put an object on the stack (a
	// trigger), which closes sorcery timing in this pre-activation model
	// even though the CR 601.2g window could have paid it.
	StackGrew bool
	Panics    int
}

// MMRealize asks whether the engine itself can pay an ordinary cast of obj.
// On clones of e it activates p's offered mana sources ("activate" options,
// every sub-decision they pose explored) and succeeds when the engine's own
// floating-pool offer walk offers the cast -- the engine never offers a
// cast its payment cannot settle. It never touches e.
//
// Two passes: first the F witness's sources only, in census order (sources
// in both censuses, then the excluded ones, mana-fee sources last), with
// only the sub-decisions branched; then, if that fails, a free search over
// every offered activation up to len(witness)+slack activations. A priority
// node whose F census can no longer pay the cast is abandoned (pruned as
// impossible). A free-search branch cut by depth or by maxNodes makes a
// negative verdict unknown.
func (e *Engine) MMRealize(p state.PlayerID, obj state.ObjID, slack, maxNodes int) MMRealizeResult {
	var res MMRealizeResult
	units, _ := e.MMUnits(p)
	budget0 := mmNodeBudget
	var wit []int
	if ok, _ := e.mmPayableW(p, e.mmCost(p, obj), e.G.Players[p].Pool, units, func(*MMAlt) bool { return true }, &budget0, &wit); !ok {
		res.Known = true
		return res
	}
	rank := func(u int) int {
		r := 3
		for _, a := range units[u].Alts {
			switch {
			case a.InW && a.InP:
				return 0
			case a.Class != "ManaFee" && r > 1:
				r = 1
			case r > 2:
				r = 2
			}
		}
		return r
	}
	var order []state.ObjID
	for want := 0; want <= 3; want++ {
		for k := len(wit) - 1; k >= 0; k-- { // witness is recorded deepest-first
			if rank(wit[k]) == want {
				order = append(order, units[wit[k]].ID)
			}
		}
	}
	budget := maxNodes
	cut := false
	stack0 := len(e.G.Stack)
	submit := func(c *Engine, in decision.Intent) (ok bool) {
		defer func() {
			if r := recover(); r != nil {
				res.Panics++
				ok = false
			}
		}()
		return c.Submit(in) == nil
	}
	var dfs func(c *Engine, depth int, guided bool) bool
	dfs = func(c *Engine, depth int, guided bool) bool {
		d := c.Pending()
		if d == nil || c.G.Over || d.Player != p {
			return false
		}
		if budget <= 0 {
			cut = true
			return false
		}
		budget--
		res.Nodes++
		if len(c.G.Stack) > stack0 {
			res.StackGrew = true
		}
		if d.Kind == decision.KPriority {
			for _, o := range d.Options {
				if o.Kind == optCast && o.Obj == obj && o.Mode == "" && o.AltCostIndex == 0 {
					return true
				}
			}
			if guided {
				if depth >= len(order) {
					return false
				}
				for _, o := range d.Options {
					if o.Kind != "activate" || o.Obj != order[depth] {
						continue
					}
					cc := c.Clone()
					if submit(cc, decision.Intent{Seq: d.Seq, Player: p, Choices: []int{o.Index}}) && dfs(cc, depth+1, true) {
						return true
					}
				}
				return false
			}
			if ok, known := c.MMFullPayable(p, obj); known && !ok {
				return false
			}
			if depth >= len(order)+slack {
				cut = true
				return false
			}
			for _, o := range d.Options {
				if o.Kind != "activate" {
					continue
				}
				cc := c.Clone()
				if !submit(cc, decision.Intent{Seq: d.Seq, Player: p, Choices: []int{o.Index}}) {
					continue
				}
				if dfs(cc, depth+1, false) {
					return true
				}
				if budget <= 0 {
					cut = true
					return false
				}
			}
			return false
		}
		// A sub-decision an activation posed: branch over single picks,
		// take the first Min indices otherwise.
		var branches [][]int
		switch {
		case d.Min <= 1 && d.Max >= 1 && len(d.Options) > 0:
			for i := range d.Options {
				if i >= 8 {
					cut = true
					break
				}
				branches = append(branches, []int{d.Options[i].Index})
			}
			if d.Min == 0 {
				branches = append(branches, []int{})
			}
		default:
			var pick []int
			for i := 0; i < d.Min && i < len(d.Options); i++ {
				pick = append(pick, d.Options[i].Index)
			}
			branches = append(branches, pick)
		}
		for _, b := range branches {
			cc := c.Clone()
			if !submit(cc, decision.Intent{Seq: d.Seq, Player: p, Choices: b}) {
				continue
			}
			if dfs(cc, depth, guided) {
				return true
			}
			if budget <= 0 {
				cut = true
				return false
			}
		}
		return false
	}
	if dfs(e.Clone(), 0, true) {
		res.Real, res.Guided, res.Known = true, true, true
		return res
	}
	cut = false
	res.Real = dfs(e.Clone(), 0, false)
	res.Known = res.Real || !cut
	return res
}
