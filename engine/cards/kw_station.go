// The Station ability's stack body (CR 702.184a). Station is an activated
// ability -- "Tap another untapped creature you control: Put a number of
// charge counters on this permanent equal to the tapped creature's power.
// Activate only as a sorcery." -- so it uses the stack. Its offer and its
// cost (the tap pick) stay rules-side (engine/rules/station.go): the cost's
// "another creature" candidate list and the tap-power read are engine
// facts. This file is only the RESOLVING body the activation mints through
// events.KeywordAbilityPush, whose Counter carries the face's K:Station line.

package cards

import "strings"

// stationAbilitySA builds the Station ability's resolving body: put CHARGE
// counters on the Spacecraft equal to X. rules binds X at resolution to the
// tapped creature's tap-power value (CR 702.184a's "the tapped creature's
// power", as a stat:TapPowerValue static such as Tapestry Warden's adjusts
// it), reading the creature the push's IDs remembered, and resolves the body
// against no face SVar table so a Spacecraft's own SVar:X cannot shadow it.
// Keyword$ Station is the action-kind tag a ValidSA$ Activated.Station scope
// reads (tapCostSAKind's precedent for Crew/Saddle). No Cost$: the cost was
// paid by the rules-side tap pick before the push.
func stationAbilitySA() *SA {
	sa, _ := parseSA("", "AB$ PutCounter | Defined$ Self | CounterType$ CHARGE | CounterNum$ X"+
		" | Keyword$ Station | SpellDescription$ Station (Tap another creature you control:"+
		" Put charge counters equal to its power on this permanent. Station only as a sorcery.)")
	return sa
}

// KeywordPushAbility is the body a KeywordAbilityPush event's Counter line
// mints: a granted keyword's activated ability (GrantedKeywordAbility), or
// the Station ability's resolving body. Station is kept OUT of
// GrantedKeywordAbility on purpose: that synthesizer also feeds the granted-
// keyword OFFER loops, and a Station body has no Cost$ -- offering it there
// would be a free activation. Every reader of a KeywordAbilityPush (the
// events.Apply mint, the activation trigger matcher) goes through this one
// function so a live game and a log-only replay agree.
func KeywordPushAbility(line string) *SA {
	if sa := GrantedKeywordAbility(line); sa != nil {
		return sa
	}
	if !strings.EqualFold(KeywordHead(line), "Station") {
		return nil
	}
	sa := stationAbilitySA()
	if sa != nil {
		sa.Params["KeywordLine"] = line
	}
	return sa
}
