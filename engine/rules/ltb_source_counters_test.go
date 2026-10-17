package rules

// ltb_source_counters_test.go pins CR 603.10a for a leaves-the-battlefield
// trigger whose effect reads its OWN source's counters through the bare
// Count$CardCounters.<KIND> head (Selfless Police Captain's "put its +1/+1
// counters on target creature you control", X = Count$CardCounters.P1P1).
// The trigger looks back in time: the count is the counters the permanent had
// on the battlefield, not the zero the card has in its new zone after Move's
// fold cleared them.

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// selflessCaptainBounced is the level-B trigger#0.0 scenario oraclediff gen
// produces for SPM: Unsummon the Captain (which entered with its
// K:etbCounter +1/+1 counter) and point the trigger at the Grizzly Bears.
const selflessCaptainBounced = `{"name":"ltb-source-counters","cr":["603.10a"],"why":"inline","setup":{"p0":{"battlefield":["Grizzly Bears","Selfless Police Captain"],"hand":["Unsummon"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[` +
	`{"op":"cast","seat":0,"card":"p0:Unsummon","mana":"U","targets":["p0:Selfless Police Captain"]},` +
	`{"op":"resolve","seat":0,"answers":[{"kind":"target","pick":["Grizzly Bears (a)"]}]}]}`

func TestLTBTriggerReadsSourceCountersLKI(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(selflessCaptainBounced))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
	// Precondition: the Captain really sat on the battlefield with its
	// +1/+1 counter, so a zero placement cannot pass as "it had none".
	setup := res.Snapshots[0]
	captain, ok := snapPerm(setup, "p0:Selfless Police Captain")
	if !ok || captain.Counters["P1P1"] != 1 {
		t.Fatalf("setup Captain should carry one +1/+1 counter: %+v (found %v)", captain, ok)
	}
	last := res.Snapshots[len(res.Snapshots)-1]
	if _, ok := snapPerm(last, "p0:Selfless Police Captain"); ok {
		t.Fatalf("the Captain is still on the battlefield; Unsummon did not resolve: %+v", last.Permanents)
	}
	bears, ok := snapPerm(last, "p0:Grizzly Bears")
	if !ok {
		t.Fatalf("p0's Grizzly Bears missing: %+v", last.Permanents)
	}
	if bears.Counters["P1P1"] != 1 || bears.PT != "3/3" {
		t.Fatalf("the LTB trigger should move the Captain's one +1/+1 counter (CR 603.10a): got %+v\n%s",
			bears, strings.Join(res.Transcript, "\n"))
	}
}
