package rules

import (
	"slices"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// XMage builds a generated scenario's starting position with
// addCard(Zone.GRAVEYARD, ...), a direct placement: a battlefield card's
// "Moved" replacement ("exile instead of graveyard") never rewrites it and a
// graveyard-watching trigger never fires for it. gorge dealt every setup card
// into the library and moved it, so Rest in Peace exiled the probe and Planar
// Void's trigger exiled it too (oracleRun.placeSetupCard).

func setupFixtureZones(t *testing.T, raw string) (gy, ex, hand []string, snap OracleSnapshot) {
	t.Helper()
	res, _ := runFixtureScenario(t, raw)
	snap = res.Snapshots[0]
	return snap.Players[0].Graveyard, snap.Players[0].Exile, snap.Players[0].Hand, snap
}

func TestSetupGraveyardPlacementBypassesMovedReplacement(t *testing.T) {
	testutil.CorpusRegistry(t)
	gy, ex, _, snap := setupFixtureZones(t,
		`{"name":"rip","setup":{"p0":{"battlefield":["Rest in Peace"],"graveyard":["Shock"]},"p1":{}},"steps":[]}`)
	// Precondition: the replacement is really in play, so an empty exile is
	// the bypass working and not a placement that never happened.
	if setupPermCount(snap, "Rest in Peace") != 1 {
		t.Fatalf("Rest in Peace is not on the battlefield at setup: %+v", snap.Permanents)
	}
	if !slices.Equal(gy, []string{"Shock"}) || len(ex) != 0 {
		t.Errorf("with Rest in Peace: graveyard=%v exile=%v, want graveyard=[Shock] exile=[]", gy, ex)
	}

	// Control: a battlefield card with no replacement gives the same zones, so
	// the two runs differ only in the card on the battlefield.
	gy2, ex2, _, snap2 := setupFixtureZones(t,
		`{"name":"bears","setup":{"p0":{"battlefield":["Grizzly Bears"],"graveyard":["Shock"]},"p1":{}},"steps":[]}`)
	if setupPermCount(snap2, "Grizzly Bears") != 1 {
		t.Fatalf("Grizzly Bears is not on the battlefield at setup: %+v", snap2.Permanents)
	}
	if !slices.Equal(gy2, gy) || len(ex2) != 0 {
		t.Errorf("control: graveyard=%v exile=%v, want graveyard=%v exile=[]", gy2, ex2, gy)
	}
}

func TestSetupGraveyardPlacementDiscardsQueuedTrigger(t *testing.T) {
	testutil.CorpusRegistry(t)
	// Planar Void is a TRIGGER, not a replacement: "Whenever another card is
	// put into a graveyard from anywhere, exile that card".
	gy, ex, _, snap := setupFixtureZones(t,
		`{"name":"void","setup":{"p0":{"battlefield":["Planar Void"],"graveyard":["Shock"]},"p1":{}},"steps":[]}`)
	if setupPermCount(snap, "Planar Void") != 1 {
		t.Fatalf("Planar Void is not on the battlefield at setup: %+v", snap.Permanents)
	}
	if !slices.Equal(gy, []string{"Shock"}) || len(ex) != 0 || len(snap.Stack) != 0 {
		t.Errorf("with Planar Void: graveyard=%v exile=%v stack=%v, want graveyard=[Shock] exile=[] stack empty",
			gy, ex, snap.Stack)
	}
}

func TestSetupOtherZonesUnchangedUnderGraveyardReplacement(t *testing.T) {
	testutil.CorpusRegistry(t)
	gy, ex, hand, snap := setupFixtureZones(t,
		`{"name":"zones","setup":{"p0":{"battlefield":["Rest in Peace"],"hand":["Shock"],"exile":["Lightning Bolt"],"graveyard":["Grizzly Bears"]},"p1":{}},"steps":[]}`)
	if setupPermCount(snap, "Rest in Peace") != 1 {
		t.Fatalf("Rest in Peace is not on the battlefield at setup: %+v", snap.Permanents)
	}
	if !slices.Equal(hand, []string{"Shock"}) || !slices.Equal(ex, []string{"Lightning Bolt"}) ||
		!slices.Equal(gy, []string{"Grizzly Bears"}) {
		t.Errorf("hand=%v exile=%v graveyard=%v, want [Shock] [Lightning Bolt] [Grizzly Bears]", hand, ex, gy)
	}
}

// Battlefield placement is unchanged: the entering card's own replacement
// (Kalonian Hydra "enters with four +1/+1 counters") still runs with Rest in
// Peace in play, and an entry trigger (Tranquil Cove's life gain) is still
// dropped by setupPlacementDropsTrigger, so life stays 20.
func TestSetupBattlefieldPlacementStillRunsEntryReplacement(t *testing.T) {
	testutil.CorpusRegistry(t)
	_, _, _, snap := setupFixtureZones(t,
		`{"name":"hydra","setup":{"p0":{"battlefield":["Rest in Peace","Kalonian Hydra","Tranquil Cove"]},"p1":{}},"steps":[]}`)
	var hydra *OracleSnapPerm
	for i := range snap.Permanents {
		if snap.Permanents[i].Name == "Kalonian Hydra" {
			hydra = &snap.Permanents[i]
		}
	}
	if hydra == nil || setupPermCount(snap, "Tranquil Cove") != 1 {
		t.Fatalf("Kalonian Hydra / Tranquil Cove are not on the battlefield at setup: %+v", snap.Permanents)
	}
	if n := hydra.Counters["P1P1"]; n != 4 {
		t.Errorf("Kalonian Hydra has %d +1/+1 counters (counters %v), want 4: battlefield placement lost its enters-with-counters replacement",
			n, hydra.Counters)
	}
	if life := snap.Players[0].Life; life != 20 {
		t.Errorf("p0 life = %d, want 20 (the entry trigger is dropped by setupPlacementDropsTrigger)", life)
	}
}

// The bypass is setup-only: a graveyard move DURING the scenario with Rest in
// Peace in play is still replaced.
func TestSetupGraveyardBypassDoesNotLeakIntoPlay(t *testing.T) {
	testutil.CorpusRegistry(t)
	res, _ := runFixtureScenario(t, `{"name":"leak","setup":{"p0":{"battlefield":["Rest in Peace"],"hand":["Shock"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"cast","seat":0,"card":"p0:Shock","mana":"R","targets":["p1:Grizzly Bears"]},{"op":"resolve","seat":0}]}`)
	if len(res.Snapshots) < 2 {
		t.Fatalf("want a setup and a post-resolve snapshot, got %d", len(res.Snapshots))
	}
	if setupPermCount(res.Snapshots[0], "Grizzly Bears") != 1 {
		t.Fatalf("Grizzly Bears is not on the battlefield at setup: %+v", res.Snapshots[0].Permanents)
	}
	last := res.Snapshots[len(res.Snapshots)-1]
	if setupPermCount(last, "Grizzly Bears") != 0 {
		t.Fatalf("Grizzly Bears survived Shock: %+v", last.Permanents)
	}
	p1 := last.Players[1]
	if !slices.Equal(p1.Exile, []string{"Grizzly Bears"}) || len(p1.Graveyard) != 0 {
		t.Errorf("p1 exile=%v graveyard=%v after Shock with Rest in Peace, want exile=[Grizzly Bears] graveyard=[]",
			p1.Exile, p1.Graveyard)
	}
}

// A hand-written (non-fixture) scenario keeps the ordinary move: its
// expectations were written against the replacement running at setup.
func TestSetupNonFixtureGraveyardPlacementKeepsReplacement(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc, err := decodeOracleScenario([]byte(
		`{"name":"hand-written","setup":{"p0":{"battlefield":["Rest in Peace"],"graveyard":["Shock"]},"p1":{}},"steps":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if sc.xmageFixture {
		t.Fatal("decoded scenario unexpectedly carries xmageFixture")
	}
	fails, transcript, run := runOracleScenario(reg, sc)
	if len(fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", fails, strings.Join(transcript, "\n"))
	}
	p0 := run.snaps[0].Players[0]
	if !slices.Equal(p0.Exile, []string{"Shock"}) || len(p0.Graveyard) != 0 {
		t.Errorf("non-fixture: exile=%v graveyard=%v, want exile=[Shock] graveyard=[]", p0.Exile, p0.Graveyard)
	}
}
