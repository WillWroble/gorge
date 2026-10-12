//go:build manamodel

package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// mmFixture: seat 0 in its main phase with only src on the battlefield and
// Lightning Bolt in hand, a priority decision posed.
func mmFixture(t *testing.T, reg *cards.Registry, src string) (*Engine, state.ObjID) {
	t.Helper()
	c := lookup(t, reg, src)
	bolt := lookup(t, reg, "Lightning Bolt")
	e := quietBaseWith(t, reg, []*cards.Card{c, bolt})
	for _, id := range append([]state.ObjID{}, e.G.Zone(state.ZBattlefield, 0)...) {
		e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZBattlefield, To: state.ZLibrary})
	}
	addZone(t, e, 0, c, state.ZBattlefield)
	b := addHand(t, e, 0, bolt)
	e.pending = nil
	e.askPriority(0)
	return e, b
}

func mmBolt(t *testing.T, r MMProbeResult, bolt state.ObjID) MMCandidate {
	t.Helper()
	for _, c := range r.Candidates {
		if c.Obj == bolt {
			return c
		}
	}
	return MMCandidate{}
}

// TestManaModelLotusPetalIsWithheldByTheSharedCensus: Lotus Petal alone pays
// Lightning Bolt (L2: the engine offers the cast after the real activation),
// the full model sees it (L1), the shared free-cost census does not, and the
// first excluded class is a card's sacrifice.
func TestManaModelLotusPetalIsWithheldByTheSharedCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e, bolt := mmFixture(t, reg, "Lotus Petal")
	head := e.L.Head()
	r := e.MMProbe(0)
	c := mmBolt(t, r, bolt)
	if !c.Known || !c.PayF || c.PayW || c.ClassW != "Sac:card" {
		t.Fatalf("Lotus Petal: want F-payable, W-withheld, class Sac:card; got %+v (units %+v)", c, r.Units)
	}
	l2 := e.MMRealize(0, bolt, 2, 200)
	if !l2.Real || !l2.Known {
		t.Fatalf("Lotus Petal: L2 must confirm the cast is payable: %+v", l2)
	}
	if e.L.Head() != head {
		t.Fatalf("the probe moved the live engine's log head")
	}
	t.Logf("Lotus Petal: %+v, L2 %+v", c, l2)
}

// TestManaModelGemstoneMineIsWithheldByThePlanner: a SubCounter land is
// outside both the shared census and the planner's.
func TestManaModelGemstoneMineIsWithheldByThePlanner(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e, bolt := mmFixture(t, reg, "Gemstone Mine")
	c := mmBolt(t, e.MMProbe(0), bolt)
	if !c.PayF || c.PayW || c.PayP || c.ClassP != "SubCounter:card" {
		t.Fatalf("Gemstone Mine: want F-payable, W- and P-withheld, class SubCounter:card; got %+v", c)
	}
	if l2 := e.MMRealize(0, bolt, 2, 200); !l2.Real {
		t.Fatalf("Gemstone Mine: L2 must confirm: %+v", l2)
	}
}

// TestManaModelMountainIsNegative: a plain Mountain is in every census, so
// nothing is withheld and the probe never walks the hand.
func TestManaModelMountainIsNegative(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e, _ := mmFixture(t, reg, "Mountain")
	r := e.MMProbe(0)
	// Precondition: the probe must actually have seen Mountain's mana
	// ability, or "nothing is withheld" is vacuous. A Mountain's {T}: {R}
	// is free-cost and deterministic, so it is a W unit.
	var units, inW int
	for i := range r.Units {
		units++
		for k := range r.Units[i].Alts {
			if r.Units[i].Alts[k].InW {
				inW++
			}
		}
	}
	if units == 0 || inW == 0 {
		t.Fatalf("Mountain: the probe saw no W mana source (%d units, %d W alts): %+v", units, inW, r.Units)
	}
	if r.ExcludedW || r.ExcludedP {
		t.Fatalf("Mountain: no alternative may be outside W or P: %+v", r.Units)
	}
}

// TestManaModelChromaticSphereIsAProxyFalsePositive: the full model ignores
// Chromatic Sphere's {1} fee, so alone it "pays" Bolt (L1), but the engine
// cannot activate it with no other mana (L2 refutes).
func TestManaModelChromaticSphereIsAProxyFalsePositive(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	e, bolt := mmFixture(t, reg, "Chromatic Sphere")
	c := mmBolt(t, e.MMProbe(0), bolt)
	if !c.PayF || c.PayW || c.ClassW != "Sac:card" {
		t.Fatalf("Chromatic Sphere: want an L1 positive; got %+v", c)
	}
	if l2 := e.MMRealize(0, bolt, 2, 200); l2.Real || !l2.Known {
		t.Fatalf("Chromatic Sphere: L2 must refute: %+v", l2)
	}
}
