package rules

import (
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// The GRANTED static replacement effect (DFT Far Fortune, End Boss' max-speed
// "a source you control would deal damage to an opponent or a permanent an
// opponent controls deals that much damage plus 1 instead"; Vnwxt, Verbose
// Host's max-speed "if you would draw a card, draw two cards instead").
//
// A static's AddReplacementEffect$ grant lives on a layer-derived
// ContinuousEffect (state.ContinuousEffect.AddRepl, written by
// engine/rules/layers.go's static scan). It cannot ride the Effect-created
// replacement scan in applyReplacementsDispatch: that scan is gated by
// effectReplacementsPossible, a latch only an AddContinuous registration
// sets, so on a fresh engine it is skipped entirely. It is instead collected
// here, the way grantedPreventMatches (engine/rules/replacement_prevent_kw.go)
// collects the granted prevention keyword: after the printed face-Repl scan,
// appended to the same match list, so ordering, CR 616.1 composition and the
// replacement context cannot drift between a printed and a granted line.
//
// The HOST is the object the granting static's Affected$ spec matches (its
// controller's frame is the replacement's "you"), resolved the same way
// grantedStatics (engine/rules/statics_granted.go) resolves a granted static's host.
// For a self grant (Affected$ Card.Self, both DFT rows) the host is the
// granting permanent itself, so ActiveZones$ Battlefield gates on that
// permanent's zone through the FULL printed matcher -- deliberately not
// replacementMatchesEffectCreated*, whose ActiveZones$ skip is right for an
// Effect's own lifetime but wrong for a source-zone-scoped granted static.
// key is "" so the match is durable across re-collections (printed-style,
// replIdentity); face is nil, the granted Repl carrying its own resolved With,
// and the match carries the GRANTING face's SVar table (grantedReplSVars(ce))
// so a CheckSVar$ gate and the ReplaceWith$ body resolve there, not from the
// host (the granting and host faces differ for a cross-object grant).
func (e *Engine) grantedStaticReplacementMatches(ev events.Event, event string) []replMatch {
	// The grant-free proof: while no object's faces carry an
	// AddReplacementEffect$ static (nor an AddStaticAbility$ grant that could
	// parse to one), no active() entry can carry AddRepl, so skip the layer
	// build entirely -- the same shape trigGrantsPossible gives the trigger
	// walk. An engine that has not proven it (a fixture-built one) scans.
	if !e.replGrantsPossible() {
		return nil
	}
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
		// The GRANTING face's SVar table: the granted R: body's CheckSVar$
		// gate and ReplaceWith$ body resolve here, NOT from the host object's
		// face. A cross-object grant (Scion of Halaster's "first draw each
		// turn" body lives on Scion; the host is the commander it affects)
		// would otherwise read the host's own missing SVar and mis-evaluate
		// the gate. See grantedReplSVars.
		svars := e.grantedReplSVars(ce)
		for _, hp := range e.G.AliveFrom(0) {
			for _, hid := range e.G.Zone(state.ZBattlefield, hp) {
				ho := e.G.Obj(hid)
				if ho == nil || ho.PhasedOut {
					continue
				}
				if !e.matchesSpecFrom(ce.Affects, hid, ce.Controller, ce.Source) {
					continue
				}
				if e.replacementMatchesGranted(*ce.AddRepl, hid, ev, svars) {
					out = append(out, replMatch{id: hid, repl: ce.AddRepl, svars: svars})
				}
			}
		}
	}
	return out
}

// grantedReplSVars resolves the SVar table a granted static replacement's
// CheckSVar$ gate and ReplaceWith$ body read: the GRANTING face's own table.
// ContinuousEffect.SVars carries it only for a merged under-card face (the
// one case where it is not the source's o.Face() table); everywhere else the
// static scan's other SVar readers fall back to the source object's face, and
// this does the same. A nil result keeps replCtx's host-face fallback.
func (e *Engine) grantedReplSVars(ce *ContinuousEffect) map[string]string {
	if ce.SVars != nil {
		return ce.SVars
	}
	if o := e.G.Obj(ce.Source); o != nil {
		if f := o.Face(); f != nil {
			return f.SVars
		}
	}
	return nil
}
