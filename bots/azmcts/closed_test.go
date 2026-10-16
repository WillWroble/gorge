package azmcts

import (
	"reflect"
	"testing"

	"github.com/adams-shaun/gorge/bots/searchprobe"
	"github.com/adams-shaun/gorge/bots/searchseat"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// testBelief is a closed-list belief over repo decks that does NOT hold the
// fixture opponent's list (mono-blue-tempo): a closed world can only deal
// the opponent cards from these.
func testBelief(t testing.TB) *searchprobe.Belief {
	t.Helper()
	reg := testutil.CorpusRegistry(t)
	var decks [][]*cards.Card
	for _, name := range []string{"uw-tempo", "dimir-tempo", "mono-green-stompy", "mono-black-aggro", "foundations-keen-engineering", "foundations-reign-of-dragons"} {
		d, err := testutil.LoadRepoDeck(reg, name)
		if err != nil {
			t.Fatal(err)
		}
		decks = append(decks, d)
	}
	b, err := searchprobe.NewBelief(decks)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// closedInput is p.input with the opponent's list closed under b.
func (p fedPosition) closedInput(t testing.TB, base *rules.Engine, b *searchprobe.Belief) RedealInput {
	in := p.input(t, base)
	in.Setup = in.Setup.Closed(0, b)
	return in
}

// Every closed world keeps what the seat sees -- the decision, every public
// zone, every zone size, its own hand and its own hidden multiset (its own
// list stays open) and every pinned known card -- while the opponent's
// hidden cards come from the belief alone, never the real list.
func TestClosedWorldsKeepWhatTheSeatSees(t *testing.T) {
	cfg := testConfig(t, "mono-red-prowess", "mono-blue-tempo", testSeed)
	p := feedPosition(t, cfg, 4, 3, nil)
	b := testBelief(t)
	head, nev := p.e.L.Head(), len(p.e.L.Events)
	in := p.closedInput(t, p.e, b)
	src, err := NewRedeal(in, searchprobe.NewCollector(0), 99, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r := src.Refused(); r != "" {
		t.Fatalf("closed redeal refused at a plain position: %s", r)
	}
	inBelief := make(map[string]bool)
	for _, n := range b.Cards().Names() {
		inBelief[n] = true
	}
	realOpp := sortedNames(p.e, p.e.G.Zone(state.ZHand, 1), p.e.G.Zone(state.ZLibrary, 1))
	hands := make(map[string]bool)
	const n = 30
	for sim := 0; sim < n; sim++ {
		w, err := src.World(sim)
		if err != nil {
			t.Fatal(err)
		}
		we := w.Engine
		if pd := we.Pending(); pd == nil || pd.Seq != p.d.Seq || pd.Kind != p.d.Kind || pd.Player != 0 {
			t.Fatal("world is not at the root decision")
		}
		for pl := state.PlayerID(0); pl < 2; pl++ {
			for _, z := range []state.Zone{state.ZBattlefield, state.ZGraveyard, state.ZExile} {
				if !reflect.DeepEqual(names(we, we.G.Zone(z, pl)), names(p.e, p.e.G.Zone(z, pl))) {
					t.Fatalf("player %d public zone %v changed", pl, z)
				}
			}
			for _, z := range []state.Zone{state.ZHand, state.ZLibrary} {
				if len(we.G.Zone(z, pl)) != len(p.e.G.Zone(z, pl)) {
					t.Fatalf("player %d zone %v size changed", pl, z)
				}
			}
		}
		if !reflect.DeepEqual(sortedNames(we, we.G.Zone(state.ZHand, 0)), sortedNames(p.e, p.e.G.Zone(state.ZHand, 0))) {
			t.Fatal("the seat's own hand changed")
		}
		if !reflect.DeepEqual(sortedNames(we, we.G.Zone(state.ZHand, 0), we.G.Zone(state.ZLibrary, 0)), sortedNames(p.e, p.e.G.Zone(state.ZHand, 0), p.e.G.Zone(state.ZLibrary, 0))) {
			t.Fatal("the seat's own hidden multiset changed: its own list must stay open")
		}
		if err := in.Known.Holds(searchprobe.World{Engine: we, Observer: p.feed.Collector()}); err != nil {
			t.Fatalf("a pinned known card moved: %v", err)
		}
		opp := sortedNames(we, we.G.Zone(state.ZHand, 1), we.G.Zone(state.ZLibrary, 1))
		if reflect.DeepEqual(opp, realOpp) {
			t.Fatal("a closed world dealt the opponent its real hidden multiset")
		}
		pinned := pinnedNames(in.Known, 1)
		for _, name := range opp {
			if !inBelief[name] && pinned[name] == 0 {
				t.Fatalf("opponent card %q is neither in the belief nor pinned", name)
			}
		}
		hands[joinNames(sortedNames(we, we.G.Zone(state.ZHand, 1)))] = true
	}
	if len(hands) < n/2 {
		t.Fatalf("only %d distinct opponent hands in %d worlds", len(hands), n)
	}
	if p.e.L.Head() != head || len(p.e.L.Events) != nev {
		t.Fatal("closed redealing changed the real engine")
	}
}

// The closed leak test: two bases identical in the seat's observation but
// holding entirely different opponent hidden cards -- the real game, and a
// closed honest root of it whose opponent hand and library are other cards
// in another partition and order -- each declared with a DIFFERENT opponent
// list, give byte-identical worlds (names and objects, position by position)
// for every seed. Neither the real list nor the real hidden cards reach a
// closed world.
func TestClosedWorldsIgnoreTheRealListAndHiddenCards(t *testing.T) {
	cfg := testConfig(t, "mono-red-prowess", "mono-blue-tempo", testSeed)
	p := feedPosition(t, cfg, 4, 3, nil)
	b := testBelief(t)
	alt, reason := searchseat.HonestRoot(p.setup.Closed(0, b), p.feed, p.e, [2]uint64{41, 42})
	if alt == nil {
		t.Fatalf("closed honest root refused: %s", reason)
	}
	realOpp := sortedNames(p.e, p.e.G.Zone(state.ZHand, 1), p.e.G.Zone(state.ZLibrary, 1))
	altOpp := sortedNames(alt, alt.G.Zone(state.ZHand, 1), alt.G.Zone(state.ZLibrary, 1))
	if reflect.DeepEqual(realOpp, altOpp) {
		t.Fatal("fixture: the two bases hold the same opponent hidden cards")
	}
	reg := testutil.CorpusRegistry(t)
	other, err := testutil.LoadRepoDeck(reg, "mono-green-stompy")
	if err != nil {
		t.Fatal(err)
	}
	ia := p.closedInput(t, p.e, b)
	ib := p.input(t, alt)
	ib.Setup.Decks = [][]*cards.Card{p.setup.Decks[0], other}
	ib.Setup = ib.Setup.Closed(0, b)
	a, err := NewRedeal(ia, searchprobe.NewCollector(0), 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := NewRedeal(ib, searchprobe.NewCollector(0), 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Refused() != "" || bb.Refused() != "" {
		t.Fatalf("refused: %q / %q", a.Refused(), bb.Refused())
	}
	for sim := 0; sim < 25; sim++ {
		wa, err := a.World(sim)
		if err != nil {
			t.Fatal(err)
		}
		wb, err := bb.World(sim)
		if err != nil {
			t.Fatal(err)
		}
		for pl := state.PlayerID(0); pl < 2; pl++ {
			for _, z := range []state.Zone{state.ZHand, state.ZLibrary} {
				za, zb := wa.Engine.G.Zone(z, pl), wb.Engine.G.Zone(z, pl)
				if !reflect.DeepEqual(names(wa.Engine, za), names(wb.Engine, zb)) {
					t.Fatalf("sim %d player %d %v names depend on the real list or hidden cards:\n%v\n%v", sim, pl, z, names(wa.Engine, za), names(wb.Engine, zb))
				}
				if pl == 1 && !reflect.DeepEqual(za, zb) {
					t.Fatalf("sim %d opponent %v objects depend on the real hidden cards:\n%v\n%v", sim, z, za, zb)
				}
			}
		}
	}
}

// An az seat on the closed world plays whole games, searches, never refuses
// or fails a deal, and replays exactly.
func TestClosedSeatPlaysAndReplays(t *testing.T) {
	cfg := testConfig(t, "mono-red-prowess", "mono-blue-tempo", testSeed)
	sc := DefaultSeatConfig()
	sc.Search.Sims, sc.World, sc.Belief = 4, WorldRedealClosed, testBelief(t)
	sc.Search.Kinds = FullKinds()
	var diags []Diag
	prev := Watch
	Watch = func(d Diag) { diags = append(diags, d) }
	t.Cleanup(func() { Watch = prev })
	heads := make([]string, 2)
	for i := range heads {
		az, err := NewSeat(cfg.Seed^1, nil, sc)
		if err != nil {
			t.Fatal(err)
		}
		if az.SearchBelief() != sc.Belief {
			t.Fatal("a closed seat does not expose its belief")
		}
		if heads[i], err = playAZ(t, cfg, az, 300); err != nil {
			t.Fatal(err)
		}
	}
	if heads[0] != heads[1] {
		t.Fatalf("replay diverged: %s vs %s", heads[0], heads[1])
	}
	var total Stats
	searched := 0
	for _, d := range diags {
		total.Add(d.Stats)
		if d.Searched {
			searched++
		}
		if d.Refused != "" || d.DealFailed != "" {
			t.Errorf("refused %q, deal failed %q", d.Refused, d.DealFailed)
		}
	}
	t.Logf("%d searched; completed %d/%d, no-world %d, refused %d", searched, total.Completed, total.Simulations, total.NoWorld, total.RedealRefused)
	if searched == 0 || total.Completed == 0 {
		t.Fatal("the closed seat never searched")
	}
}

func TestNewSeatRefusesAClosedWorldWithoutABelief(t *testing.T) {
	sc := DefaultSeatConfig()
	sc.World = WorldRedealClosed
	if _, err := NewSeat(1, nil, sc); err == nil {
		t.Fatal("a closed world without a belief was accepted")
	}
	sc.World = WorldRedeal
	sc.Belief = testBelief(t)
	s, err := NewSeat(1, nil, sc)
	if err != nil {
		t.Fatal(err)
	}
	if s.SearchBelief() != nil {
		t.Fatal("an open-list seat reports a belief")
	}
}

func pinnedNames(k searchprobe.KnownCards, p state.PlayerID) map[string]int {
	out := make(map[string]int)
	for _, h := range k.Hands {
		if h.Player == p {
			for _, c := range h.Cards {
				out[c.Name]++
			}
		}
	}
	for _, l := range k.Libraries {
		if l.Player == p {
			for _, c := range l.Members {
				out[c.Name]++
			}
		}
	}
	return out
}

func joinNames(ns []string) string {
	s := ""
	for _, n := range ns {
		s += n + "|"
	}
	return s
}
