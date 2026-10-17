package cost

import (
	"math"
	"strconv"
	"strings"

	"github.com/adams-shaun/gorge/engine/state"
)

// groupExileCost matches Forge's open-ended exile count, the "N+" form of the
// battlefield Exile<N/Spec> and graveyard ExileFromGrave<N/Spec> heads:
// Fabrication Foundry's Exile<1+/Artifact.Other+withTotalCMCEQX/...>, Baron
// Helmut Zemo's ExileFromGrave<1+/Card.Black+withTotalManaSymbols_Black_GE15>
// and The Capitoline Triad's ExileFromGrave<1+/Card.Historic+withTotalCMCGE30>
// (the whole corpus, measured by TestGroupCostCensus). The payer exiles N or
// more matching objects; the spec's withTotal* group predicate, when present,
// constrains the elected SET and is moved into CostPart.Group (see
// stripGroupTotal). Group 1 is the head suffix ("" for the battlefield form).
const groupExileCost = `^Exile(|FromGrave)<(\d+)\+/([^/>]+)(?:/([^>]*))?>$`

// chooseCreatureTypeCost matches Forge's ChooseCreatureType<N> cost token --
// "as an additional cost to cast this spell, choose a creature type" (Caller
// of the Hunt's Cost$, Celestial Reunion's OptionalCost static). Choosing is
// the payment (CR 601.2h); the choice is recorded on the spell (Object.
// ChosenType) so a Card.ChosenType filter in the same cost (Celestial
// Reunion's Behold<2/Creature.ChosenType>) and in the resolution read it.
const chooseCreatureTypeCost = `^ChooseCreatureType<(\d+)>$`

// matchGroupExileCost is ^Exile(|FromGrave)<(\d+)\+/([^/>]+)(?:/([^>]*))?>$:
// groups [sym, suffix, count, spec, desc].
func matchGroupExileCost(t costTok) (groups, bool) {
	if !t.angle || (t.id != hExile && t.id != hExileFromGrave) {
		return groups{}, false
	}
	n, spec, desc, ok := countSpecDesc(t.inner, isDigitsPlus)
	if !ok {
		return groups{}, false
	}
	return groups{t.sym, t.head[len("Exile"):], n[:len(n)-1], spec, desc}, true
}

// matchChooseCreatureTypeCost is ^ChooseCreatureType<(\d+)>$.
func matchChooseCreatureTypeCost(t costTok) (groups, bool) {
	if !t.angle || t.id != hChooseCreatureType || !isDigits(t.inner) {
		return groups{}, false
	}
	return groups{t.sym, t.inner}, true
}

// isDigitsPlus is \d+\+.
func isDigitsPlus(s string) bool {
	return len(s) >= 2 && s[len(s)-1] == '+' && isDigits(s[:len(s)-1])
}

// appendGroupExileCost records one matched groupExileCost token as an AtLeast
// Exile part. A malformed count or an unrecognised withTotal* predicate is
// the reported one-generic fallback every other malformed token takes: a
// group predicate left in the spec would be an unknown per-object term that
// fails every candidate, i.e. a silently unpayable cost.
func appendGroupExileCost(c *Cost, sym string, m groups) {
	n, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || n < 0 || n > int64(math.MaxInt32) {
		c.Generic = AddClampedGeneric(c.Generic, 1)
		c.reportUnknown(sym)
		return
	}
	spec, group, ok := stripGroupTotal(strings.ReplaceAll(m[3], ";", ","))
	if !ok {
		c.Generic = AddClampedGeneric(c.Generic, 1)
		c.reportUnknown(sym)
		return
	}
	part := CostPart{N: int32(n), Spec: spec, Desc: m[4], AtLeast: true, Group: group, Zone: state.ZBattlefield}
	if m[1] == "FromGrave" {
		part.Zone = state.ZGraveyard
	}
	c.Exile = append(c.Exile, part)
}

// groupTotalPrefix opens every set-level sum predicate Forge writes. The
// withTotalPowerGE<N> floor is NOT read here: it rides the tapXType head
// (stripGroupPowerFloor / CostPart.MinPower).
const groupTotalPrefix = "withTotal"

// stripGroupTotal splits a withTotal* group predicate off an AtLeast part's
// spec. It returns the spec without the predicate, the parsed constraint, and
// ok=false only when a withTotal* term is present but not one this grammar
// reads. A spec with no such term returns unchanged with the zero GroupTotal.
// The terms are '+'-joined conjuncts of the (single-alternative) spec.
func stripGroupTotal(spec string) (string, GroupTotal, bool) {
	if !strings.Contains(spec, groupTotalPrefix) {
		return spec, GroupTotal{}, true
	}
	if strings.Contains(spec, ",") {
		// A comma alternative would need the predicate per alternative; no
		// carrier writes one.
		return spec, GroupTotal{}, false
	}
	// The terms follow the base after '.', or after '+' when the spec
	// carries no '.' (Artifact+withTotal...).
	base, rest, found := strings.Cut(spec, ".")
	if !found {
		base, rest, _ = strings.Cut(spec, "+")
	}
	var keep []string
	var g GroupTotal
	for term := range strings.SplitSeq(rest, "+") {
		if !strings.HasPrefix(term, groupTotalPrefix) {
			keep = append(keep, term)
			continue
		}
		if g.Active() {
			return spec, GroupTotal{}, false
		}
		parsed, ok := parseGroupTotal(term)
		if !ok {
			return spec, GroupTotal{}, false
		}
		g = parsed
	}
	if len(keep) == 0 || (len(keep) == 1 && keep[0] == "") {
		return base, g, true
	}
	return base + "." + strings.Join(keep, "+"), g, true
}

// groupColorWords maps the colour word of a withTotalManaSymbols_<Color>_ term
// to its WUBRG letter.
var groupColorWords = map[string]byte{"White": 'W', "Blue": 'U', "Black": 'B', "Red": 'R', "Green": 'G'}

// parseGroupTotal reads one withTotal* term:
//
//	withTotalCMC<Cmp><Rhs>                  (withTotalCMCEQX, withTotalCMCGE30)
//	withTotalManaSymbols_<Color>_<Cmp><Rhs> (withTotalManaSymbols_Black_GE15)
//
// Cmp is EQ or GE; Rhs a literal or the announced name X.
func parseGroupTotal(term string) (GroupTotal, bool) {
	g := GroupTotal{Raw: term}
	body := strings.TrimPrefix(term, groupTotalPrefix)
	switch {
	case strings.HasPrefix(body, "CMC"):
		g.Prop = GroupCMC
		body = body[len("CMC"):]
	case strings.HasPrefix(body, "ManaSymbols_"):
		g.Prop = GroupManaSymbols
		color, cmp, found := strings.Cut(body[len("ManaSymbols_"):], "_")
		letter, known := groupColorWords[color]
		if !found || !known {
			return GroupTotal{}, false
		}
		g.Color, body = letter, cmp
	default:
		return GroupTotal{}, false
	}
	if len(body) < 3 {
		return GroupTotal{}, false
	}
	switch body[:2] {
	case "EQ":
		g.Cmp = GroupEQ
	case "GE":
		g.Cmp = GroupGE
	default:
		return GroupTotal{}, false
	}
	rhs := body[2:]
	if isDigits(rhs) {
		v, err := strconv.ParseInt(rhs, 10, 32)
		if err != nil {
			return GroupTotal{}, false
		}
		g.N = int32(v)
		return g, true
	}
	if rhs != "X" {
		return GroupTotal{}, false
	}
	g.Var = rhs
	return g, true
}

// FormatGroupSpec re-joins a part's group predicate onto its spec, the
// inverse of stripGroupTotal (FormatCost's spelling).
func FormatGroupSpec(part CostPart) string {
	if !part.Group.Active() {
		return part.Spec
	}
	if strings.Contains(part.Spec, ".") {
		return part.Spec + "+" + part.Group.Raw
	}
	return part.Spec + "." + part.Group.Raw
}

// ManaSymbolsOfColor counts the mana symbols of colour letter (WUBRG) in a
// parsed mana cost: plain pips, hybrid pips with that colour on either face,
// Phyrexian pips of that colour, monocolour hybrid {2/C} pips and hybrid
// Phyrexian pips. Each such symbol is ONE symbol of that colour (CR 107.4e,
// 107.4f; CR 202.2a's colour of a symbol). Generic and X contribute nothing.
func ManaSymbolsOfColor(c Cost, letter byte) int32 {
	n := c.Colored[state.ManaIndex(letter)]
	for _, h := range c.Hybrid {
		if h.A == letter || h.B == letter {
			n++
		}
	}
	for _, p := range c.Phyrexian {
		if p == letter {
			n++
		}
	}
	for _, t := range c.Twobrid {
		if t.Col == letter {
			n++
		}
	}
	for _, hp := range c.HybridPhyrexian {
		if hp.A == letter || hp.B == letter {
			n++
		}
	}
	return n
}
