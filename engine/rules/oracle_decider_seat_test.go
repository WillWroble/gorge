package rules

// B8: OptionalDecider$ TriggeredSourceController resolved its decider from
// Remembered[0], which for a DamageDone trigger is the DAMAGED object -- so
// Brood Sliver's may was asked of Brood Sliver's controller instead of the
// controller of the Sliver that dealt the damage. TestOptional...Seat pins the
// engine-level fix through the recorded decision's Seat; TestOracleAnswer...
// pins the scenario runner's own seat assertion, which is what lets the
// regenerated xmageport scenario catch the regression with no hand-written
// check of its own.

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

const broodSliverSeatScenario = `{
  "name": "brood-sliver-decider-seat",
  "setup": {
    "p0": {"battlefield": ["Venom Sliver"]},
    "p1": {"battlefield": ["Brood Sliver"]}
  },
  "steps": [
    {"op": "attack", "seat": 0, "attackers": ["p0:Venom Sliver"], "defender": "p1"},
    {"op": "pass_to", "seat": 0, "step": "combat-damage", "active": "p0"},
    {"op": "resolve", "seat": 0, "answers": [{"kind": "any", "pick": ["yes"]}]}
  ],
  "expect": [
    {"life": {"p1": 19}},
    {"count": {"seat": 0, "zone": "battlefield", "name": "Sliver", "token": true}, "eq": 1},
    {"count": {"seat": 1, "zone": "battlefield", "name": "Sliver", "token": true}, "eq": 0}
  ]
}`

func TestOptionalDeciderTriggeredSourceControllerSeat(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc, err := decodeOracleScenario([]byte(broodSliverSeatScenario))
	if err != nil {
		t.Fatal(err)
	}
	sc.xmageFixture = true
	fails, transcript, run := runOracleScenario(reg, sc)
	if len(fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", fails, strings.Join(transcript, "\n"))
	}
	var seats []int
	for i := range run.decisions {
		if run.decisions[i].GorgeKind == string(decision.KTriggerOptional) {
			seats = append(seats, run.decisions[i].Seat)
		}
	}
	if len(seats) != 1 || seats[0] != 0 {
		t.Fatalf("Brood Sliver's may was posed to %v, want exactly one ask of p0 (the damaging Venom Sliver's controller)\n%s",
			seats, strings.Join(transcript, "\n"))
	}
}

func TestOracleAnswerSeatAssertion(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc, err := decodeOracleScenario([]byte(broodSliverSeatScenario))
	if err != nil {
		t.Fatal(err)
	}
	sc.xmageFixture = true
	wrong := 1
	sc.Steps[2].Answers[0].Seat = &wrong
	fails, transcript, _ := runOracleScenario(reg, sc)
	if len(fails) == 0 {
		t.Fatalf("answer asserted seat 1; runner did not report a seat mismatch\n%s", strings.Join(transcript, "\n"))
	}
	if !strings.Contains(fails[0], "posed to p0, scenario says p1") {
		t.Fatalf("seat mismatch failure did not name both seats: %v", fails)
	}
}
