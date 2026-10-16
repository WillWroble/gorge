package events

import "github.com/adams-shaun/gorge/engine/state"

// foldHiddenIdentity gives a hidden card the identity the event names (see
// HiddenIdentity): card Amount of the game's hypothetical card table, face 0.
// An event that does not name a hand, library or sideboard card of its
// Player, or whose ordinal does not name Text in the table, changes nothing;
// the emitter (rules.ReidentifyHidden) checks the card landed, so that is
// never silent.
func foldHiddenIdentity(g *state.Game, e *Event) {
	u := g.HypotheticalCards
	o := g.Obj(e.Obj)
	if u == nil || o == nil || o.Owner != e.Player || (o.Zone != state.ZHand && o.Zone != state.ZLibrary && o.Zone != state.ZSideboard) {
		return
	}
	if e.Amount < 0 || int(e.Amount) >= u.Len() || u.Name(int(e.Amount)) != e.Text {
		return
	}
	o.SetCard(u.Card(int(e.Amount)), 0)
}
