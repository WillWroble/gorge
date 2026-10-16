package rules

import (
	"fmt"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/deck"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// ReidentifyHidden gives hidden cards of a HYPOTHETICAL engine (a search
// world: CloneHypothetical, NewHypothetical) new card identities: objs[i]
// becomes card ords[i] of u, face 0. Every object must be a card in its
// owner's hand, library or sideboard. It is the closed-decklist redeal's
// re-identification (bots/searchprobe): a world in which an opponent's
// unknown cards are drawn from a belief over deck lists rather than
// permuted from the real ones.
//
// Each change is one Secret events.HiddenIdentity on e's own log, so a
// tape re-run of the world re-applies it like any redeal event. u becomes
// the engine's hypothetical card table (state.Game.HypotheticalCards, set
// once; a second, different table is refused), and every card dealt raises
// the engine's pool gates exactly as a minted out-of-pool card does. A real
// match engine is refused: a match never changes a card's identity.
func ReidentifyHidden(e *Engine, u *cards.Universe, objs []state.ObjID, ords []int32) error {
	if e == nil || e.rng == nil || e.rng.chance == nil {
		return fmt.Errorf("rules: ReidentifyHidden requires a hypothetical engine")
	}
	if len(objs) != len(ords) {
		return fmt.Errorf("rules: ReidentifyHidden: %d objects, %d cards", len(objs), len(ords))
	}
	if len(objs) == 0 {
		return nil
	}
	if u == nil {
		return fmt.Errorf("rules: ReidentifyHidden: no card table")
	}
	if cur := e.G.HypotheticalCards; cur != nil && cur != u {
		return fmt.Errorf("rules: ReidentifyHidden: the engine already holds another card table")
	}
	e.G.HypotheticalCards = u
	for i, id := range objs {
		o := e.G.Obj(id)
		if o == nil || o.IsToken || o.IsCopy || (o.Zone != state.ZHand && o.Zone != state.ZLibrary && o.Zone != state.ZSideboard) {
			return fmt.Errorf("rules: ReidentifyHidden: object %d is not a hand, library or sideboard card", id)
		}
		ord := ords[i]
		if ord < 0 || int(ord) >= u.Len() {
			return fmt.Errorf("rules: ReidentifyHidden: card %d outside the table", ord)
		}
		c := u.Card(int(ord))
		events.Emit(e.G, e.L, events.Event{Kind: events.HiddenIdentity, Player: o.Owner, Obj: id, Amount: ord, Text: u.Name(int(ord)), Secret: true})
		if o = e.G.Obj(id); o.Card != c || o.FaceIdx != 0 {
			return fmt.Errorf("rules: ReidentifyHidden: object %d did not take card %q", id, u.Name(int(ord)))
		}
		e.notePoolCard(c)
		e.forgetWalkClass(id)
	}
	return nil
}

// forgetWalkClass drops id's cached offer-walk class (walkObjCls). The
// cache is refreshed from the log before it is read, so this changes no
// answer; it only keeps a re-identified object's PREVIOUS card -- in a
// closed search world, the opponent's real hidden card -- from lingering in
// the world as the stale entry's fingerprint.
func (e *Engine) forgetWalkClass(id state.ObjID) {
	e.ownWalkClasses()
	if i := int(id) - 1; i >= 0 && i < len(e.walkObjCls) {
		e.walkObjCls[i] = walkObjClass{}
	}
}

// ClosedDeckManifests is e's genesis manifest table (OwnDeck's source) as a
// seat that does not know its opponents' lists may hold it: keep's own
// manifest, shared and read-only, and an empty manifest for every other
// seat. The closed-decklist redeal installs it on every world it deals
// (UseDeckManifests), so a closed world's OwnDeck(opponent) -- and every
// Board or View an opponent-perspective simulation builds from it -- names
// nothing of the opponent's real list or sideboard.
func (e *Engine) ClosedDeckManifests(keep state.PlayerID) []deck.Manifest {
	out := make([]deck.Manifest, len(e.deckManifests))
	if int(keep) >= 0 && int(keep) < len(out) {
		out[keep] = e.deckManifests[keep]
	}
	return out
}

// UseDeckManifests replaces a HYPOTHETICAL engine's genesis manifest table
// with ms (ClosedDeckManifests), shared and never written. The table is
// observer data that no event reads, so the world's play is unchanged; a
// real match engine is refused, as ReidentifyHidden refuses one.
func (e *Engine) UseDeckManifests(ms []deck.Manifest) error {
	if e == nil || e.rng == nil || e.rng.chance == nil {
		return fmt.Errorf("rules: UseDeckManifests requires a hypothetical engine")
	}
	if len(ms) != len(e.deckManifests) {
		return fmt.Errorf("rules: UseDeckManifests: %d manifests for %d seats", len(ms), len(e.deckManifests))
	}
	e.deckManifests = ms
	return nil
}
