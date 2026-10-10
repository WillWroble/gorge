package rules

import (
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// The GRANTED static replacement effect (DFT Far Fortune, End Boss' max-speed
// "a source you control would deal damage to an opponent or a permanent an
// opponent controls deals that much damage plus 1 instead"; Vnwxt, Verbose
// Host's max-speed "if you would draw a card, draw two cards instead").
//
// A static's AddReplacementEffect$ grant lives on a layer-derived
// ContinuousEffect (state.ContinuousEffect.AddRepl, written by
// rules/layers.go's static scan). It cannot ride the Effect-created
// replacement scan in applyReplacementsDispatch: that scan is gated by
// effectReplacementsPossible, a latch only an AddContinuous registration
// sets, so on a fresh engine it is skipped entirely. It is instead collected
// here, the way grantedPreventMatches (rules/replacement_prevent_kw.go)
// collects the granted prevention keyword: after the printed face-Repl scan,
// appended to the same match list, so ordering, CR 616.1 composition and the
// replacement context cannot drift between a printed and a granted line.
//
// The HOST is the object the granting static's Affected$ spec matches (its
// controller's frame is the replacement's "you"), resolved the same way
// grantedStatics (rules/statics_granted.go) resolves a granted static's host.
// For a self grant (Affected$ Card.Self, both DFT rows) the host is the
// granting permanent itself, so ActiveZones$ Battlefield gates on that
// permanent's zone through the FULL printed matcher -- deliberately not
// replacementMatchesEffectCreated*, whose ActiveZones$ skip is right for an
// Effect's own lifetime but wrong for a source-zone-scoped granted static.
// key is "" so the match is durable across re-collections (printed-style,
// replIdentity); face is nil, the granted Repl carrying its own resolved With.
func (e *Engine) grantedStaticReplacementMatches(ev events.Event, event string) []replMatch {
	// Read the active() list ONCE into a local: the host walk below must not
	// re-enter active()'s build (its depth bracket in activeBuild).
	act := e.active()
	var out []replMatch
	for i := range act {
		ce := &act[i]
		if ce.AddRepl == nil {
			continue
		}
		// A cheap event-class prefilter before enumerating hosts: a granted
		// DamageDone replacement never needs a Draw's battlefield walk.
		if !replacementEventNameMatches(ce.AddRepl.Event, event) {
			continue
		}
		for _, hp := range e.G.AliveFrom(0) {
			for _, hid := range e.G.Zone(state.ZBattlefield, hp) {
				ho := e.G.Obj(hid)
				if ho == nil || ho.PhasedOut {
					continue
				}
				if !e.matchesSpecFrom(ce.Affects, hid, ce.Controller, ce.Source) {
					continue
				}
				if e.replacementMatches(*ce.AddRepl, hid, ev) {
					out = append(out, replMatch{id: hid, repl: ce.AddRepl})
				}
			}
		}
	}
	return out
}
