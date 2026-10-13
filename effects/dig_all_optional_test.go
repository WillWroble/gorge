package effects

import (
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

const digAllOptionalSA = "DB$ Dig | Defined$ You | DigNum$ 3 | ChangeNum$ All | Optional$ True | PromptToSkipOptionalAbility$ True | ChangeValid$ Land | DestinationZone$ Exile"

// digAllBoard puts [land, bear, land] on seat 0's library, tape-served with
// the given option index (-1 unserved).
func digAllBoard(t *testing.T, answer int) (*endureTapeHost, []state.ObjID) {
	t.Helper()
	h := &endureTapeHost{fakeHost: newHost(t, 2), answer: answer}
	bear := mkCard(t, "Name:Bear\nTypes:Creature\nPT:2/2\nOracle:x\n")
	land := mkCard(t, "Name:Isle\nTypes:Basic Land Island\nOracle:x\n")
	ids := []state.ObjID{h.g.AddObject(land, 0).ID, h.g.AddObject(bear, 0).ID, h.g.AddObject(land, 0).ID}
	h.g.SetZone(state.ZLibrary, 0, ids)
	if !MatchesSpecCtx(h.g, "Land", ids[0], NewSpecContext(0, 0)) || !MatchesSpecCtx(h.g, "Land", ids[2], NewSpecContext(0, 0)) ||
		MatchesSpecCtx(h.g, "Land", ids[1], NewSpecContext(0, 0)) {
		t.Fatal("precondition: two lands and a non-land in the window")
	}
	return h, ids
}

// A yes/no election: ChangeNum$ All with Optional$ takes every matching card
// or none, so the ask is two options (yes first), never a subset pick.
func TestDigAllOptionalAsksYesNo(t *testing.T) {
	h, ids := digAllBoard(t, 0)
	var posed *decision.Decision
	Resolve(&recordAsk{endureTapeHost: h, out: &posed}, &Ctx{Controller: 0}, sa(t, digAllOptionalSA))
	if posed == nil {
		t.Fatal("no decision posed")
	}
	if posed.ResumeKind != "dig_all" || posed.Min != 1 || posed.Max != 1 || len(posed.Options) != 2 ||
		posed.Options[0].Kind != "yes" || posed.Options[1].Kind != "no" {
		t.Fatalf("decision = %+v, want a yes/no (Min 1 Max 1)", posed)
	}
	if ex := h.g.Zone(state.ZExile, 0); len(ex) != 2 {
		t.Fatalf("yes: exile = %v, want both lands", ex)
	}
	if lib := h.g.Zone(state.ZLibrary, 0); len(lib) != 1 || lib[0] != ids[1] {
		t.Fatalf("yes: library = %v, want only the bear", lib)
	}
}

func TestDigAllOptionalDeclineMovesNothing(t *testing.T) {
	h, _ := digAllBoard(t, 1)
	Resolve(h, &Ctx{Controller: 0}, sa(t, digAllOptionalSA))
	if h.asks == 0 {
		t.Fatal("precondition: the election was never posed")
	}
	if ex := h.g.Zone(state.ZExile, 0); len(ex) != 0 {
		t.Fatalf("no: exile = %v, want empty", ex)
	}
	if lib := h.g.Zone(state.ZLibrary, 0); len(lib) != 3 {
		t.Fatalf("no: library = %v, want untouched", lib)
	}
}

// recordAsk captures the decision a tape-served ask poses.
type recordAsk struct {
	*endureTapeHost
	out **decision.Decision
}

func (r *recordAsk) TapeAnswer(d *decision.Decision) (decision.Intent, bool) {
	cp := *d
	*r.out = &cp
	return r.endureTapeHost.TapeAnswer(d)
}

// digAllOptionalCarriers pins every corpus card whose Dig carries ChangeNum$
// All with an optional election: each is a yes/no, not a subset pick.
var digAllOptionalCarriers = []string{
	"A-Lantern of Revealing", "Amulet of Quoz", "Expedited Inheritance", "Explorer's Scope",
	"Hans Eriksson", "Lantern of Revealing", "Raiders' Karve", "Rebirth", "Timmerian Fiends",
}

func TestDigAllOptionalCensus(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	got := map[string]bool{}
	visit := func(name string, s *cards.SA) {
		if s == nil || s.API != "Dig" {
			return
		}
		dp := DigOf(s)
		if dp.ChangeNumAll && (dp.Optional || dp.PromptToSkipOptional) {
			got[name] = true
		}
	}
	for _, card := range reg.AllCards() {
		for _, f := range card.Faces {
			for _, s := range f.Abilities {
				visit(f.Name, s)
			}
			names := make([]string, 0, len(f.SVars))
			for n := range f.SVars {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				if strings.Contains(f.SVars[n], "Dig") {
					visit(f.Name, cards.ResolveSVar(f.SVars, n))
				}
			}
		}
	}
	var have []string
	for n := range got {
		have = append(have, n)
	}
	sort.Strings(have)
	if strings.Join(have, "|") != strings.Join(digAllOptionalCarriers, "|") {
		t.Errorf("carriers = %q\nwant %q", have, digAllOptionalCarriers)
	}
}
