package view

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/state"
)

// TestCardViewProjectsPrintedPT pins CardView.PrintedPT: the token's PRINTED
// P/T, the fact the token-art cache key is derived from, distinct from the
// CURRENT (layer-7) Power/Toughness a counter or anthem moves. A face with no
// printed P/T projects an empty string, and a redacted face-down card reveals
// neither.
func TestCardViewProjectsPrintedPT(t *testing.T) {
	g := state.NewGame([]string{"a", "b"})
	soldier, d := cards.ParseBytes("s.txt", []byte("Name:Soldier Token\nTypes:Creature Soldier\nPT:1/1\nOracle:x\n"))
	if len(d) != 0 {
		t.Fatalf("diags: %v", d)
	}
	soldier.Link()
	tok := g.AddObject(soldier, 0)
	tok.IsToken = true
	tok.Zone = state.ZBattlefield
	g.SetZone(state.ZBattlefield, 0, []state.ObjID{tok.ID})

	// Precondition: the face really carries the printed P/T the projection is
	// asserted against — a fixture whose face printed nothing would make the
	// assertion below vacuous.
	if tok.Face().PT != "1/1" {
		t.Fatalf("fixture face printed P/T = %q, want 1/1", tok.Face().PT)
	}

	v := Project(g, nil, 0, nil)
	var cv CardView
	for _, c := range v.Players[0].Battlefield {
		if c.ID == tok.ID {
			cv = c
		}
	}
	if cv.ID != tok.ID {
		t.Fatalf("token %d missing from the projected battlefield: %+v", tok.ID, v.Players[0].Battlefield)
	}
	if cv.PrintedPT != "1/1" {
		t.Fatalf("CardView.PrintedPT = %q, want the face's printed 1/1", cv.PrintedPT)
	}
}
