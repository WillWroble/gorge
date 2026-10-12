package events

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"

	"github.com/adams-shaun/gorge/state"
)

// TestTurnWatchersFoldAndReset drives each watcher through events.Apply
// and checks the per-seat tallies, the infect exclusion, seat isolation and
// the TurnChange reset.
func TestTurnWatchersFoldAndReset(t *testing.T) {
	g, l := twoPlayer(t)
	id := g.Zone(state.ZLibrary, 0)[0]
	g.Tokens = map[string]*cards.Card{"Bear": bearCard()}
	p0, p1 := &g.Players[0], &g.Players[1]
	if p0.LifeGainedThisTurn != 0 || p1.LifeLostThisTurn != 0 {
		t.Fatal("watchers must start at zero")
	}

	Emit(g, l, Event{Kind: PutOnStack, Obj: id, Player: 0, From: state.ZLibrary, To: state.ZStack})
	Emit(g, l, Event{Kind: LifeChange, Player: 0, Amount: 3})
	Emit(g, l, Event{Kind: LifeChange, Player: 0, Amount: -2})
	Emit(g, l, Event{Kind: Damage, Player: 1, Amount: 4})
	Emit(g, l, Event{Kind: Damage, Player: 1, Amount: 5, Counter: "infect"})
	Emit(g, l, Event{Kind: TokenCreate, Player: 1, Text: "Bear"})

	if p0.SpellsCastThisTurn != 1 || p1.SpellsCastThisTurn != 0 {
		t.Errorf("spells cast = %d/%d, want 1/0", p0.SpellsCastThisTurn, p1.SpellsCastThisTurn)
	}
	if p0.LifeGainedThisTurn != 3 || p1.LifeGainedThisTurn != 0 {
		t.Errorf("life gained = %d/%d, want 3/0", p0.LifeGainedThisTurn, p1.LifeGainedThisTurn)
	}
	if p0.LifeLostThisTurn != 2 || p1.LifeLostThisTurn != 4 {
		t.Errorf("life lost = %d/%d, want 2/4 (infect is poison, not loss)", p0.LifeLostThisTurn, p1.LifeLostThisTurn)
	}
	if p0.TokensCreatedThisTurn != 0 || p1.TokensCreatedThisTurn != 1 {
		t.Errorf("tokens = %d/%d, want 0/1", p0.TokensCreatedThisTurn, p1.TokensCreatedThisTurn)
	}
	if c := g.Clone(); c.Players[1].LifeLostThisTurn != 4 {
		t.Errorf("clone lost the tally: %d", c.Players[1].LifeLostThisTurn)
	}

	Emit(g, l, Event{Kind: TurnChange, Player: 1, Amount: 2})
	for i := range g.Players {
		p := g.Players[i]
		if p.SpellsCastThisTurn+p.LifeGainedThisTurn+p.LifeLostThisTurn+p.TokensCreatedThisTurn != 0 {
			t.Errorf("seat %d watchers not reset at TurnChange: %+v", i, p)
		}
	}
}
