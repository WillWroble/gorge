package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// hovershipEngine deals seat 0 Unidentified Hovership and Shatter (plus
// basics). With stageBear, seat 1 has a Grizzly Bears on the battlefield for
// the ETB to exile; without it there is no creature anywhere, so the ETB's
// "up to one target creature" exiles nothing.
func hovershipEngine(t *testing.T, reg *cards.Registry, stageBear bool) (*Engine, Config, state.ObjID) {
	t.Helper()
	plains := searchCorpusCard(t, reg, "Plains")
	mountain := searchCorpusCard(t, reg, "Mountain")
	bear := searchCorpusCard(t, reg, "Grizzly Bears")
	deck := []*cards.Card{
		searchCorpusCard(t, reg, "Unidentified Hovership"),
		searchCorpusCard(t, reg, "Shatter"),
	}
	for len(deck) < 40 {
		deck = append(deck, plains, mountain)
	}
	opp := []*cards.Card{bear}
	for len(opp) < 40 {
		opp = append(opp, mountain)
	}
	cfg := seatZeroStart(Config{Seed: 9211, Names: []string{"hovership", "opponent"},
		Decks: [][]*cards.Card{deck, opp}, Tokens: reg.Tokens})
	e := New(cfg)
	e.Advance()
	toMain1(t, e)
	bearID := state.ObjID(0)
	if stageBear {
		for _, z := range []state.Zone{state.ZHand, state.ZLibrary} {
			for _, id := range e.G.Zone(z, 1) {
				if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Grizzly Bears" && bearID == 0 {
					e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: z, To: state.ZBattlefield})
					e.pending = nil
					bearID = id
				}
			}
		}
		if bearID == 0 {
			t.Fatal("no Grizzly Bears in seat 1's hand/library to stage")
		}
		e.priorityRound()
	}
	return e, cfg, bearID
}

// hovershipEnterAndShatter casts the Hovership (answering its ETB with the
// bear when one is staged, with no target otherwise), resolves the ETB, then
// destroys the Hovership with Shatter and drives until the leave trigger has
// resolved or asks something. It returns the Hovership's id and the first
// non-priority decision (nil when the stack emptied at priority).
func hovershipEnterAndShatter(t *testing.T, e *Engine, bearID state.ObjID) (state.ObjID, *decision.Decision) {
	t.Helper()
	hov := searchMoveByName(t, e, "Unidentified Hovership", state.ZHand)
	addMana(t, e, 0, "WWW")
	castByID(t, e, hov)
	d := passUntilResolved(t, e, 20)
	if d != nil && d.Kind == decision.KTarget {
		if bearID == 0 {
			t.Fatalf("ETB target ask with no creature on the battlefield: %+v", d)
		}
		idx := -1
		for _, o := range d.Options {
			if o.Obj == bearID {
				idx = o.Index
			}
		}
		if idx < 0 {
			t.Fatalf("staged bear %d not offered to the ETB exile: %+v", bearID, d.Options)
		}
		submitChoices(t, e, idx)
	} else if bearID != 0 {
		t.Fatalf("after Hovership entered: %+v, want the ETB exile target ask", d)
	}
	if d = passUntilResolved(t, e, 20); d == nil || d.Kind != decision.KPriority {
		t.Fatalf("while the ETB resolved: %+v, want priority", d)
	}
	if o := e.G.Obj(hov); o == nil || o.Zone != state.ZBattlefield {
		t.Fatalf("Hovership = %+v, want battlefield", o)
	}
	if bearID != 0 {
		if o := e.G.Obj(bearID); o == nil || o.Zone != state.ZExile {
			t.Fatalf("exiled bear = %+v, want exile", o)
		}
	}

	shatter := searchMoveByName(t, e, "Shatter", state.ZHand)
	addMana(t, e, 0, "RRR")
	castByID(t, e, shatter)
	if d = e.Pending(); d == nil || d.Kind != decision.KTarget {
		t.Fatalf("after casting Shatter: %+v, want the target ask", d)
	}
	tIdx := -1
	for _, o := range d.Options {
		if o.Obj == hov {
			tIdx = o.Index
		}
	}
	if tIdx < 0 {
		t.Fatalf("Hovership %d not offered to Shatter: %+v", hov, d.Options)
	}
	submitChoices(t, e, tIdx)
	d = passUntilResolved(t, e, 20)
	if o := e.G.Obj(hov); o == nil || o.Zone != state.ZGraveyard {
		t.Fatalf("destroyed Hovership = %+v, want graveyard", o)
	}
	if d != nil && d.Kind == decision.KPriority {
		return hov, nil
	}
	return hov, d
}

// castByID submits seat 0's cast option for id.
func castByID(t *testing.T, e *Engine, id state.ObjID) {
	t.Helper()
	d := e.Pending()
	idx := -1
	for _, o := range d.Options {
		if o.Kind == "cast" && o.Obj == id {
			idx = o.Index
		}
	}
	if idx < 0 {
		t.Fatalf("no cast option for %d: %+v", id, d.Options)
	}
	submitChoices(t, e, idx)
}

// hovershipLeaveTriggerResolved reports whether a TrigDread instance whose
// source is the Hovership resolved off the stack -- the
// precondition that makes "nothing was manifested" meaningful.
func hovershipLeaveTriggerResolved(e *Engine, hov state.ObjID) bool {
	for _, ev := range e.L.Events {
		if ev.Kind != events.Resolve {
			continue
		}
		if o := e.G.Obj(ev.Obj); o != nil && o.Source == hov && o.Ability != nil && o.Ability.API == "ManifestDread" {
			return true
		}
	}
	return false
}

func manifestedDreadObjects(e *Engine) []state.ObjID {
	var out []state.ObjID
	for p := range e.G.Players {
		for _, id := range e.G.Zone(state.ZBattlefield, state.PlayerID(p)) {
			if o := e.G.Obj(id); o != nil && o.FaceDown {
				out = append(out, id)
			}
		}
	}
	return out
}

// TestUnidentifiedHovershipNothingExiledManifestsNothing pins CR 608.2b on
// `DefinedPlayer$ RememberedOwner`: the Hovership's ETB exiled nothing, so
// "the exiled card's owner" names no player and nobody manifests dread. The
// leave trigger's own capture (the Hovership) is not a remembered card; the
// old read handed it back, so the Hovership's owner manifested dread.
func TestUnidentifiedHovershipNothingExiledManifestsNothing(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	e, cfg, _ := hovershipEngine(t, reg, false)
	// Draw both spells into hand first so the library baseline only moves
	// if a manifest takes cards from it.
	searchMoveByName(t, e, "Unidentified Hovership", state.ZHand)
	searchMoveByName(t, e, "Shatter", state.ZHand)
	lib0 := len(e.G.Zone(state.ZLibrary, 0))
	hov, d := hovershipEnterAndShatter(t, e, 0)
	if d != nil {
		t.Fatalf("leave trigger asked %+v (player %d); nothing was exiled, so nobody manifests dread", d.Kind, d.Player)
	}
	if !hovershipLeaveTriggerResolved(e, hov) {
		t.Fatal("precondition: the Hovership's leave trigger never resolved")
	}
	if fd := manifestedDreadObjects(e); len(fd) != 0 {
		t.Fatalf("face-down manifested objects %v; nothing was exiled", fd)
	}
	if got := len(e.G.Zone(state.ZLibrary, 0)); got != lib0 {
		t.Fatalf("seat 0 library %d -> %d: the Hovership's controller manifested dread", lib0, got)
	}
	for _, ev := range e.L.Events {
		if ev.Kind == events.Note && strings.Contains(ev.Text, "unimplemented API ManifestDread") {
			t.Fatalf("ManifestDread degraded to a note: %q", ev.Text)
		}
	}
	replayCheck(t, e, cfg)
}

// TestUnidentifiedHovershipExiledCardOwnerManifests is the positive half:
// the ETB exiled seat 1's bear, so seat 1 (the exiled card's owner) -- not
// the Hovership's controller -- is the one asked to manifest dread.
func TestUnidentifiedHovershipExiledCardOwnerManifests(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	e, _, bear := hovershipEngine(t, reg, true)
	_, d := hovershipEnterAndShatter(t, e, bear)
	if d == nil || d.Kind != decision.KChoose || d.ResumeKind != "manifest_dread" {
		t.Fatalf("after the leave trigger: %+v, want seat 1's manifest-dread choice", d)
	}
	if d.Player != 1 {
		t.Fatalf("manifest dread asked player %d, want 1 (the exiled bear's owner)", d.Player)
	}
}
