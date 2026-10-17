package rules

import (
	"reflect"
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// TestPotentialWalkSharedAcrossReaders pins the posed decision's shared
// potential walk (potential_walk_cache.go): once a full-walk reader has asked
// on the engine, the offer builder, the PotentialActions projection and
// PotentialPaymentPlans together open exactly ONE legal-action walk per
// decision, and each answers exactly what an engine that never cached
// answers (a Clone with its cache emptied, at every read: a clone carries
// its parent's current entry, adoptPotentialWalk, so the cold reference
// drops it).
func TestPotentialWalkSharedAcrossReaders(t *testing.T) {
	prevPriced, prevCasts, prevWalk := pricedCandidatesVerify, castsOnlyWalkVerify, walkCacheVerify
	pricedCandidatesVerify, castsOnlyWalkVerify, walkCacheVerify = false, false, false
	defer func() { pricedCandidatesVerify, castsOnlyWalkVerify, walkCacheVerify = prevPriced, prevCasts, prevWalk }()
	e := onePassBenchBoard(t)
	fresh := func() *Engine {
		c := e.Clone()
		c.potentialWalk = potentialWalkCache{}
		return c
	}
	wantActs := fresh().PotentialActions(0)
	wantPlans := fresh().PotentialPaymentPlans(0)

	// First decision: no full-walk reader has asked yet, so the builder runs
	// its own casts-only walk and the projection then needs the full one.
	e.EnsurePaymentActions()
	walks := e.legalActionWalks
	if got := e.PotentialActions(0); !reflect.DeepEqual(got, wantActs) {
		t.Fatalf("PotentialActions = %+v, want %+v", got, wantActs)
	}
	if got := e.legalActionWalks - walks; got != 1 {
		t.Fatalf("first projection opened %d walks, want 1 (the full walk)", got)
	}
	walks = e.legalActionWalks
	if got := e.PotentialPaymentPlans(0); !reflect.DeepEqual(got, wantPlans) {
		t.Fatalf("PotentialPaymentPlans = %+v, want %+v", got, wantPlans)
	}
	// The planner's own witness confirmations walk other pools; only the
	// PotentialMana walk is shared, so compare against a cold engine's count.
	c := fresh()
	cw := c.legalActionWalks
	c.PotentialPaymentPlans(0)
	if got, cold := e.legalActionWalks-walks, c.legalActionWalks-cw; got != cold-1 {
		t.Fatalf("PotentialPaymentPlans opened %d walks after the projection, want %d (a cold engine's %d less the shared one)", got, cold-1, cold)
	}

	// Next decision at the same board: the builder now runs the full walk,
	// and the projection and planner are served it.
	onePassReask(e, 0)
	want := fresh().PaymentActionsForPriority(0, e.Pending().Seq)
	walks = e.legalActionWalks
	got := e.EnsurePaymentActions()
	if n := e.legalActionWalks - walks; n != 1 {
		t.Fatalf("builder opened %d walks, want 1", n)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payment actions = %+v, want %+v", got, want)
	}
	walks = e.legalActionWalks
	if got := e.PotentialActions(0); !reflect.DeepEqual(got, wantActs) {
		t.Fatalf("served PotentialActions = %+v, want %+v", got, wantActs)
	}
	if n := e.legalActionWalks - walks; n != 0 {
		t.Fatalf("projection after the builder opened %d walks, want 0", n)
	}

	// A direct pool write (no event) misses rather than serving stale.
	e.G.Players[0].Pool[state.ManaIndex('R')] = 1
	cold := fresh().PotentialActions(0)
	if got := e.PotentialActions(0); !reflect.DeepEqual(got, cold) {
		t.Fatalf("after a pool write PotentialActions = %+v, want %+v", got, cold)
	}
}

// TestPotentialWalkCarriesAcrossClone pins adoptPotentialWalk: a clone of an
// engine whose potential walk is current at the posed decision carries it,
// so the clone's first potential readers open no walk and re-plan nothing,
// and answer exactly what a cold clone (its cache emptied) computes; a
// clone taken once the parent's entry is stale (a later decision) starts
// cold, as before.
func TestPotentialWalkCarriesAcrossClone(t *testing.T) {
	prevPriced, prevCasts, prevWalk := pricedCandidatesVerify, castsOnlyWalkVerify, walkCacheVerify
	pricedCandidatesVerify, castsOnlyWalkVerify, walkCacheVerify = false, false, false
	defer func() { pricedCandidatesVerify, castsOnlyWalkVerify, walkCacheVerify = prevPriced, prevCasts, prevWalk }()
	e := onePassBenchBoard(t)
	cold := func() *Engine {
		c := e.Clone()
		c.potentialWalk = potentialWalkCache{}
		return c
	}
	// The parent's builder and projection fill the entry (the full walk and
	// every plain cast's plan).
	acts := e.EnsurePaymentActions()
	if len(acts) < 4 {
		t.Fatalf("precondition: the board offers %d payment actions, want >= 4", len(acts))
	}
	wantActs := e.PotentialActions(0)
	if !e.potentialWalkCarries() {
		t.Fatal("the parent's entry is not current at its posed decision")
	}
	c := e.Clone()
	if c.potentialWalk.opts == nil || !c.potentialWalkHit(0, true) {
		t.Fatal("the clone did not carry the parent's current entry")
	}
	if &c.potentialWalk.opts[0] == &e.potentialWalk.opts[0] {
		t.Fatal("the clone shares the parent's option array")
	}
	walks := c.legalActionWalks
	if got := c.PotentialActions(0); !reflect.DeepEqual(got, wantActs) {
		t.Fatalf("carried PotentialActions = %+v, want %+v", got, wantActs)
	}
	if n := c.legalActionWalks - walks; n != 0 {
		t.Fatalf("the clone's projection opened %d walks, want 0", n)
	}
	if got, want := c.PotentialPaymentPlans(0), cold().PotentialPaymentPlans(0); !reflect.DeepEqual(got, want) {
		t.Fatalf("carried PotentialPaymentPlans = %+v, want %+v", got, want)
	}
	// The submitted plan's validation is served the carried plan: no walk.
	var a *decision.PaymentAction
	for i := range acts {
		if len(acts[i].Plans) > 0 {
			a = &acts[i]
			break
		}
	}
	if a == nil {
		t.Fatal("precondition: no payable cast")
	}
	c2 := e.Clone()
	walks = c2.legalActionWalks
	if err := c2.ValidateCastPayment(0, a.Cast, a.Plans[0]); err != nil {
		t.Fatalf("carried validation: %v", err)
	}
	if n := c2.legalActionWalks - walks; n != 0 {
		t.Fatalf("the clone's validation opened %d walks, want 0", n)
	}
	if err := cold().ValidateCastPayment(0, a.Cast, a.Plans[0]); err != nil {
		t.Fatalf("cold validation: %v", err)
	}
	// A new decision on the parent: its entry is stale, so a clone starts
	// cold and its projection opens a walk.
	onePassReask(e, 0)
	if e.potentialWalkCarries() {
		t.Fatal("a stale entry reported as carriable")
	}
	c3 := e.Clone()
	if c3.potentialWalk.opts != nil {
		t.Fatal("a clone of a stale entry carried it")
	}
	walks = c3.legalActionWalks
	c3.PotentialActions(0)
	if n := c3.legalActionWalks - walks; n == 0 {
		t.Fatal("a cold clone's projection opened no walk")
	}
}
