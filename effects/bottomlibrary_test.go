package effects

import (
	"strconv"
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// TestJaceRealitySculptorUltimateKeepsOnlyBottomCard drives the ACTUAL
// compiled corpus SA for Jace, Reality Sculptor's [0] ultimate ("Exile all
// but the bottom card of each opponent's library", ChangeType$
// Card.OppOwn+!BottomLibrary). Before BottomLibrary was a recognised word the
// filter was unknown and failed closed, so the ability exiled nothing.
func TestJaceRealitySculptorUltimateKeepsOnlyBottomCard(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	jace, ok := reg.Lookup("Jace, Reality Sculptor")
	if !ok {
		t.Fatal("corpus has no Jace, Reality Sculptor")
	}
	var ab *cards.SA
	for _, a := range jace.Faces[0].Abilities {
		if a.API == "ChangeZoneAll" && a.Params["ChangeType"] == "Card.OppOwn+!BottomLibrary" {
			ab = a
			break
		}
	}
	if ab == nil {
		t.Fatal("Jace, Reality Sculptor has no Card.OppOwn+!BottomLibrary ChangeZoneAll")
	}

	h := newHost(t, 2)
	for i := 0; i < 39; i++ {
		lzFiller(t, h, 1, "O"+strconv.Itoa(i))
	}
	mine := lzFills(t, h, 3)
	if lib := h.g.Zone(state.ZLibrary, 1); len(lib) != 39 {
		t.Fatalf("precondition: opponent library = %d cards, want 39", len(lib))
	}
	bottom := h.g.Zone(state.ZLibrary, 1)[38]

	Resolve(h, &Ctx{Controller: 0, Source: mine[0]}, ab)

	lib := h.g.Zone(state.ZLibrary, 1)
	if len(lib) != 1 || lib[0] != bottom {
		t.Fatalf("opponent library = %v, want only the bottom card %v", lib, bottom)
	}
	if n := len(h.g.Zone(state.ZExile, 1)); n != 38 {
		t.Fatalf("opponent exile = %d cards, want 38", n)
	}
	if n := len(h.g.Zone(state.ZLibrary, 0)); n != 3 {
		t.Fatalf("controller library = %d cards, want 3 (OppOwn)", n)
	}
	for _, e := range h.log {
		if e.Kind == events.Note {
			t.Fatalf("unexpected Note %+v", e)
		}
	}
}

// TestBottomLibraryMatchesOnlyLastLibraryCard pins the word itself: only the
// last card of the owner's library matches; a deeper card or a card outside
// the library does not.
func TestBottomLibraryMatchesOnlyLastLibraryCard(t *testing.T) {
	h := newHost(t, 2)
	ids := lzFills(t, h, 3)
	if UnknownPredicates("Card.BottomLibrary") != nil {
		t.Fatalf("BottomLibrary must be a recognised word, got %v", UnknownPredicates("Card.BottomLibrary"))
	}
	if !MatchesObjectCtx(h.g, "Card.BottomLibrary", h.g.Obj(ids[2]), SpecContext{You: 0}) {
		t.Errorf("the last library card must match BottomLibrary")
	}
	for _, id := range ids[:2] {
		if MatchesObjectCtx(h.g, "Card.BottomLibrary", h.g.Obj(id), SpecContext{You: 0}) {
			t.Errorf("library card %v above the bottom must not match", id)
		}
	}
	gy := lzCreature(t, h, 0, state.ZGraveyard, "G")
	if MatchesObjectCtx(h.g, "Card.BottomLibrary", h.g.Obj(gy), SpecContext{You: 0}) {
		t.Errorf("a graveyard card must not match BottomLibrary")
	}
}
