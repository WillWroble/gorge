package rules

import (
	"fmt"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// ReidentifyHidden gives hidden cards of a HYPOTHETICAL engine (a search
// world: CloneHypothetical, NewHypothetical) new card identities: objs[i]
// becomes card ords[i] of u, face 0. Every object must be a card in its
// owner's hand or library. It is the closed-decklist redeal's
// re-identification (internal/searchprobe): a world in which an opponent's
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
		if o == nil || o.IsToken || o.IsCopy || (o.Zone != state.ZHand && o.Zone != state.ZLibrary) {
			return fmt.Errorf("rules: ReidentifyHidden: object %d is not a hand or library card", id)
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
	}
	return nil
}
