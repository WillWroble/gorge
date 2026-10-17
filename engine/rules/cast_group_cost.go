package rules

import (
	"fmt"
	"strconv"

	"github.com/adams-shaun/gorge/engine/decision"
	costvocab "github.com/adams-shaun/gorge/engine/rules/cost"
	"github.com/adams-shaun/gorge/engine/rules/pay"
	"github.com/adams-shaun/gorge/engine/state"
)

// cast_group_cost.go settles the open-ended "N or more" exile cost parts
// (CostPart.AtLeast) and their set-level withTotal* constraint
// (CostPart.Group): Fabrication Foundry's "exile one or more other artifacts
// you control with total mana value X", Baron Helmut Zemo's "exile any number
// of black cards from your graveyard with fifteen or more black mana symbols
// among their mana costs", The Capitoline Triad's "historic cards ... with
// total mana value 30 or greater".
//
// The election is ONE KChoose over every candidate: Min is the part's N, Max
// every candidate, each option's Value the candidate's contribution
// (pay.GroupValue), and the constraint rides the decision's cumulative
// budget -- a GE floor is Decision.MinSum, an EQ total is MinSum and a
// present MaxSum together -- so Decision.Validate enforces the set rule on
// every answer and a rules-ignorant client can grey out a busting pick.

// groupExileAsk poses the election of one AtLeast Exile part. candidates are
// exAsk's already-filtered candidates for the part (spec, zone, cost block,
// distinct from earlier parts). An election the board can no longer satisfy
// aborts the proposal (CR 733.1: nothing has moved yet).
func (e *Engine) groupExileAsk(pc *pendingCast, part CostPart, zones []state.Zone, candidates []state.ObjID) bool {
	rhs := part.Group.N
	if part.Group.Var != "" {
		rhs = pc.x
	}
	values := pay.GroupValues(e.G, candidates, part.Group)
	if !pay.GroupFeasible(values, int(part.N), part.Group, rhs) {
		e.abortCast(pc, "exile cost no longer payable; cast/activation aborted", true)
		return true
	}
	zoneName := "battlefield"
	if len(zones) != 0 && zones[0] == state.ZGraveyard {
		zoneName = "graveyard"
	}
	prompt := "Exile " + strconv.Itoa(int(part.N)) + " or more card(s) from your " + zoneName
	if part.Group.Active() {
		prompt += fmt.Sprintf(" (total %s)", groupTotalPhrase(part, rhs))
	}
	d := &decision.Decision{Player: pc.player, Kind: decision.KChoose, Min: int(part.N), Max: len(candidates),
		Prompt: prompt + " to activate " + e.targetName(pc.card), Source: pc.card}
	if !pc.isAbility() {
		d.Prompt = prompt + " to cast " + e.targetName(pc.card)
	}
	if part.Group.Active() {
		d.MinSum = int(rhs)
		if part.Group.Cmp == costvocab.GroupEQ {
			d.MaxSum, d.Budgeted = int(rhs), true
		}
	}
	for i, id := range candidates {
		d.Options = append(d.Options, decision.Option{Index: len(d.Options), Kind: "exilecost",
			Obj: id, Label: e.G.Obj(id).Face().Name, Value: int(values[i])})
	}
	e.choosing = chooseCast
	e.ask(d)
	return true
}

// groupTotalPhrase renders a part's set constraint for a prompt.
func groupTotalPhrase(part CostPart, rhs int32) string {
	what := "mana value"
	if part.Group.Prop == costvocab.GroupManaSymbols {
		what = string(part.Group.Color) + " mana symbols"
	}
	cmp := "exactly "
	if part.Group.Cmp == costvocab.GroupGE {
		cmp = "at least "
	}
	return what + " " + cmp + strconv.Itoa(int(rhs))
}

// groupAnnounceX lists the announcements an X-bound group part admits
// (withTotalCMCEQX, Fabrication Foundry's Announce$ X): every total some
// "N or more" subset of the part's candidates reaches (pay.GroupSums), so the
// announced X is always one the exile election can settle (CR 601.2b). ok is
// false when the cost carries no such part.
func (e *Engine) groupAnnounceX(pc *pendingCast) ([]int32, bool) {
	for _, part := range pc.cost.Exile {
		if !part.AtLeast || part.Group.Var == "" {
			continue
		}
		var candidates []state.ObjID
		for _, zone := range exilePartZones(part) {
			for _, oid := range pay.ExileCostCandidates(e.G, zone, pc.player, part) {
				if !pc.isAbility() && oid == pc.card {
					continue
				}
				if zone == state.ZBattlefield && e.exileBlockedForCost(oid, pay.CostCauseForAbility(pc.isAbility())) {
					continue
				}
				if e.matchesSpecFrom(part.Spec, oid, pc.player, pc.card) {
					candidates = append(candidates, oid)
				}
			}
		}
		return pay.GroupSums(pay.GroupValues(e.G, candidates, part.Group), int(part.N)), true
	}
	return nil, false
}

// groupAnnounceXAsk is xAsk's arm for an X announced by an X-bound group
// part and nothing else (no {X} mana symbol, no other announced count -- the
// only corpus carrier, Fabrication Foundry, is this shape). The offered
// values are the reachable totals that also leave the ability a legal target
// (its ValidTgts$ reads the same X: "target artifact card with mana value X
// or less"), so neither the target ask nor the election can strand the
// announcement. handled is false when the arm does not apply.
func (e *Engine) groupAnnounceXAsk(pc *pendingCast) (handled bool) {
	if pc.cost.X > 0 {
		return false
	}
	sums, ok := e.groupAnnounceX(pc)
	if !ok {
		return false
	}
	sa := e.pcAbility(pc)
	var vals []int32
	// targetSpecContext reads e.cast.x, so bind each candidate while the
	// target census runs (xTargetPotentialMods' discipline); pc.x is assigned
	// the announcement only when the decision is answered.
	prevX := pc.x
	for _, x := range sums {
		pc.x = x
		if sa == nil || e.targetSAAvailable(pc.player, pc.card, pc.card, sa, x, false) {
			vals = append(vals, x)
		}
	}
	pc.x = prevX
	if len(vals) == 0 {
		e.abortCast(pc, "announce cost no longer payable; cast/activation aborted", true)
		return true
	}
	d := &decision.Decision{Player: pc.player, Kind: decision.KChoose, Min: 1, Max: 1,
		Prompt: "Choose a value for X", Source: pc.card}
	for _, x := range vals {
		d.Options = append(d.Options, decision.Option{Index: len(d.Options), Kind: "x",
			Label: fmt.Sprintf("X = %d", x), Amount: int(x)})
	}
	e.choosing = chooseCast
	e.ask(d)
	return true
}

// costLinksGroupExiles reports whether a cost's exiled cards are linked to
// the paying source (Object.ExiledWith, the MoveZone IDs payload): an
// ability whose cost carries an AtLeast group part reads the exiled set back
// as "those exiled cards" -- Baron Helmut Zemo's Defined$ ExiledWith copies
// (CR 607.2a's linked "exiled this way"; Forge's CostExile records the set on
// the host the same way). Every carrier has exactly one Exile part, so the
// link names that part's election.
func costLinksGroupExiles(c Cost) bool {
	for _, part := range c.Exile {
		if part.AtLeast {
			return true
		}
	}
	return false
}
