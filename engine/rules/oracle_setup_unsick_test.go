package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// oracleSetupUnsickScenario seats a {T}-ability creature with the
// NON-starting seat and activates it on p0's turn 1. XMage's addCard places
// setup permanents as if they had been there since before the game, so the
// ability is activatable (CR 302.6 reads "under its controller's control
// continuously since their most recent turn began" -- for a harness-seeded
// permanent the harness stands in for that history). Before SetupSettled the
// first TurnChange cleared sickness only for p0's permanents and the
// activation was "not offered" (the combat/BecomesBlockedTest Trap Runner
// rows in gorge-compliance).
const oracleSetupUnsickScenario = `{"name":"setup-nonstarting-seat-unsick","cr":["302.6"],"why":"p1's setup Prodigal Pyromancer taps on p0's turn 1",
 "setup":{"p0":{"battlefield":["Grizzly Bears"]},"p1":{"battlefield":["Prodigal Pyromancer"]}},
 "steps":[{"op":"pass","seat":0},
  {"op":"activate","seat":1,"card":"p1:Prodigal Pyromancer","targets":["p0"]},
  {"op":"resolve","seat":1,"expect":[{"life":{"p0":19}},{"card":"p1:Prodigal Pyromancer","tapped":true}]}]}`

func TestOracleSetupNonStartingSeatPermanentIsNotSummoningSick(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(oracleSetupUnsickScenario))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
}
