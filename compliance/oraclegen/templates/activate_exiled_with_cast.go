package templates

import (
	"strings"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/compliance/oraclegen"
)

// An ability that targets a card "exiled with" its source, where the source
// itself does the exiling AS IT ENTERS from its controller's graveyard
// (Mimeoplasm, Revered One: "As Mimeoplasm enters, exile up to X creature
// cards from your graveyard"; "{2}: Mimeoplasm becomes a copy of target
// creature card exiled with it"). Setup placement runs no replacement and an
// exile placement is bound to nothing (CR 406.6), so the only honest
// arrangement is the real cast: the source starts in hand with one creature
// card in the graveyard, is cast for X = 1, and the as-enters exile picks that
// card, which the activation then targets.

// exiledWithCastCard is the graveyard creature card the source exiles: a
// vanilla creature whose name no baseline permanent shares, so XMage binds
// the exile target by name unambiguously.
const exiledWithCastCard = "Hill Giant"

// exiledWithCastPrelude moves the source to p0's hand, seeds the graveyard
// card, and returns the cast-and-resolve steps plus the activation's target;
// ok is false for any ability or face outside the shape.
func exiledWithCastPrelude(f *cards.Face, sa *cards.SA, name string, p0 *oraclegen.Seat) ([]oraclegen.Step, []string, bool) {
	if !exiledWithCastShape(f, sa) {
		return nil, nil, false
	}
	pool, why := oraclegen.PoolFor(strings.Replace(f.ManaCost, "X", "1", 1))
	if why != "" {
		return nil, nil, false
	}
	for i, n := range p0.Battlefield {
		if n == name {
			p0.Battlefield = append(p0.Battlefield[:i:i], p0.Battlefield[i+1:]...)
			break
		}
	}
	// The battlefield-placement toughness floor (a 0/0 seeded with +1/+1
	// counters) does not apply: the cast's own replacement sets them.
	if p0.Counters[name] != nil {
		p0.Counters = cloneSeatCounters(p0.Counters)
		delete(p0.Counters, name)
	}
	p0.Hand = appendFixtureUnique(p0.Hand, name)
	p0.Graveyard = appendFixtureUnique(p0.Graveyard, exiledWithCastCard)
	ref := "p0:" + exiledWithCastCard
	return []oraclegen.Step{
		{Op: "cast", Seat: 0, Card: "p0:" + name, Mana: pool,
			Answers: []oraclegen.Answer{{Kind: "choose", Pick: []string{"X = 1"}}}},
		{Op: "resolve", Answers: []oraclegen.Answer{{Kind: "choose", Pick: []string{exiledWithCastCard}}}},
	}, []string{ref}, true
}

// exiledWithCastShape reports the shape: the ability's only target is a
// Creature.ExiledWithSource card in exile, the face's mana cost carries {X},
// and its as-enters replacement exiles creature cards from the graveyard,
// remembering them (the ExiledWith binding).
func exiledWithCastShape(f *cards.Face, sa *cards.SA) bool {
	if sa == nil || !strings.EqualFold(strings.TrimSpace(sa.ParamStr(cards.PKValidTgts)), "Creature.ExiledWithSource") ||
		!strings.EqualFold(strings.TrimSpace(sa.Params["TgtZone"]), "Exile") ||
		!strings.Contains(" "+f.ManaCost+" ", " X ") {
		return false
	}
	for _, r := range f.Repls {
		if !strings.EqualFold(r.Event, "Moved") || !strings.EqualFold(r.Params["Destination"], "Battlefield") ||
			!strings.EqualFold(r.Params["ValidCard"], "Card.Self") {
			continue
		}
		body := f.SVars[r.Params["ReplaceWith"]]
		if strings.Contains(body, "DB$ ChangeZone") && strings.Contains(body, "Origin$ Graveyard") &&
			strings.Contains(body, "Destination$ Exile") && strings.Contains(body, "RememberChanged$ True") {
			return true
		}
	}
	return false
}

// cloneSeatCounters copies a setup counter map so a fixture's shared map is
// never mutated in place.
func cloneSeatCounters(m map[string]map[string]int32) map[string]map[string]int32 {
	out := make(map[string]map[string]int32, len(m))
	for card, kinds := range m {
		out[card] = kinds
	}
	return out
}

// faceDownTargetProbe is the Disguise card a "target face-down permanent"
// ability aims at: cast face down for {3} in the prelude (CR 702.168a), it is
// the only face-down permanent on the board.
const faceDownTargetProbe = "Bolrac-Clan Basher"

// faceDownTargetPrelude serves an ability whose only target is a face-down
// permanent (Hauntwoods Shrieker's "Reveal target face-down permanent"). No
// setup op places a permanent face down, so the prelude casts the Disguise
// probe face down from hand and the activation targets it. ok is false for
// any other ability.
func faceDownTargetPrelude(reg *cards.Registry, sa *cards.SA, p0 *oraclegen.Seat) ([]oraclegen.Step, []string, bool) {
	if sa == nil || !faceDownTargetShape(sa) {
		return nil, nil, false
	}
	c, ok := reg.Lookup(faceDownTargetProbe)
	if !ok || len(c.Faces) == 0 {
		return nil, nil, false
	}
	_, _, mode, _, why := morphFamilyCost(c.Faces[0])
	if why != "" {
		return nil, nil, false
	}
	p0.Hand = appendFixtureUnique(p0.Hand, faceDownTargetProbe)
	ref := "p0:" + faceDownTargetProbe
	return []oraclegen.Step{
		{Op: "cast", Seat: 0, Card: ref, Mana: faceDownCastPool, CastMode: mode},
		{Op: "resolve"},
	}, []string{ref}, true
}

// faceDownTargetShape reports an ability whose one target filter is a bare
// face-down card or permanent.
func faceDownTargetShape(sa *cards.SA) bool {
	switch strings.TrimSpace(sa.ParamStr(cards.PKValidTgts)) {
	case "Card.faceDown", "Permanent.faceDown", "Creature.faceDown":
		return true
	}
	return false
}

// castFixturePrelude is the activate template's one hook for a target the
// fixture cross product cannot place and a real cast must make: the source's
// own exiled-with cast, the source's own token-and-secret-choice cast (A
// Killer Among Us), or a face-down probe. ok is false when neither shape
// holds.
func castFixturePrelude(reg *cards.Registry, f *cards.Face, sa *cards.SA, name string, p0 *oraclegen.Seat) ([]oraclegen.Step, []string, bool) {
	if steps, targets, ok := exiledWithCastPrelude(f, sa, name, p0); ok {
		return steps, targets, true
	}
	if steps, targets, ok := revealChosenTokenPrelude(f, sa, name, p0); ok {
		return steps, targets, true
	}
	return faceDownTargetPrelude(reg, sa, p0)
}

// castFixtureShape reports whether castFixturePrelude serves the ability.
func castFixtureShape(f *cards.Face, sa *cards.SA) bool {
	_, reveal := revealChosenTokenShape(f, sa)
	return exiledWithCastShape(f, sa) || reveal || (sa != nil && faceDownTargetShape(sa))
}

// revealChosenTokenShape reads an ability whose cost reveals the source's
// secretly chosen creature type and sacrifices the source, targeting an
// attacking creature token (A Killer Among Us). The source's own enters
// trigger makes the tokens and makes the secret choice among ValidTypes$;
// chosen is the first type, whose token the prelude attacks with. ok is false
// for any other ability.
func revealChosenTokenShape(f *cards.Face, sa *cards.SA) (chosen string, ok bool) {
	if sa == nil || !strings.Contains(sa.ParamStr(cards.PKCost), "RevealChosen<Type/") ||
		!strings.EqualFold(strings.TrimSpace(sa.ParamStr(cards.PKValidTgts)), "Creature.token+attacking") {
		return "", false
	}
	for _, t := range f.Triggers {
		if t.Mode != "ChangesZone" || !strings.EqualFold(t.ParamStr(cards.PKValidCard), "Card.Self") {
			continue
		}
		body := f.SVars[t.Params["Execute"]]
		sub := f.SVars[subAbilityOf(body)]
		if !strings.Contains(body, "DB$ Token") || !strings.Contains(sub, "DB$ ChooseType") || !strings.Contains(sub, "Secretly$ True") {
			continue
		}
		for _, part := range strings.Split(sub, "|") {
			if v, has := strings.CutPrefix(strings.TrimSpace(part), "ValidTypes$"); has {
				first := strings.TrimSpace(strings.Split(v, ",")[0])
				return first, first != ""
			}
		}
	}
	return "", false
}

// subAbilityOf is an SVar body's SubAbility$ name, "" when it has none.
func subAbilityOf(body string) string {
	for _, part := range strings.Split(body, "|") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "SubAbility$"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// revealChosenTokenPrelude casts the source, answers its secret choice with
// the first offered type, advances to p0's next turn (a token made this turn
// cannot attack, CR 302.6) and attacks with that type's token, which the
// activation then targets.
func revealChosenTokenPrelude(f *cards.Face, sa *cards.SA, name string, p0 *oraclegen.Seat) ([]oraclegen.Step, []string, bool) {
	chosen, ok := revealChosenTokenShape(f, sa)
	if !ok {
		return nil, nil, false
	}
	pool, why := oraclegen.PoolFor(f.ManaCost)
	if why != "" {
		return nil, nil, false
	}
	for i, n := range p0.Battlefield {
		if n == name {
			p0.Battlefield = append(p0.Battlefield[:i:i], p0.Battlefield[i+1:]...)
			break
		}
	}
	p0.Hand = appendFixtureUnique(p0.Hand, name)
	token := "p0:token:" + chosen + " Token"
	return []oraclegen.Step{
		{Op: "cast", Seat: 0, Card: "p0:" + name, Mana: pool},
		{Op: "resolve", Answers: []oraclegen.Answer{{Kind: "choose", Pick: []string{chosen}}}},
		{Op: "pass_to", Seat: 0, Step: "upkeep", Active: "p0"},
		{Op: "pass_to", Seat: 0, Step: "main1", Active: "p0"},
		{Op: "attack", Seat: 0, Defender: "p1", Attackers: []string{token}},
	}, []string{token}, true
}
