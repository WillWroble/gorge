package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// A scripted declaration that omits a creature which must attack (CR 508.1d)
// is completed by the harness, as XMage's game forces the required attacker
// whatever its test script declared (gorge-compliance
// continuous/GideonJuraAndRowanKenrithNextTurnTest). Juggernaut attacks each
// combat if able: a pass_to through combat and an explicit empty attack
// both declare it, and its 5 damage connects.
const oracleRequiredAttackerPassToScenario = `{"name":"required-attacker-pass-to","cr":["508.1d"],"why":"a pass_to through combat declares the Juggernaut that must attack",
 "setup":{"p0":{"battlefield":["Juggernaut"]}},
 "steps":[{"op":"pass_to","seat":0,"step":"main2","active":"p0"}],
 "expect":[{"life":{"p1":15}},{"card":"p0:Juggernaut","tapped":true}]}`

const oracleRequiredAttackerEmptyAttackScenario = `{"name":"required-attacker-empty-attack","cr":["508.1d"],"why":"an empty scripted attack still declares the Juggernaut that must attack",
 "setup":{"p0":{"battlefield":["Juggernaut"]}},
 "steps":[{"op":"attack","seat":0,"attackers":[],"defender":"p1"},
  {"op":"pass_to","seat":0,"step":"end","active":"p0"},{"op":"resolve","seat":0}],
 "expect":[{"life":{"p1":15}}]}`

func TestOracleRequiredAttackerIsForced(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	for _, sc := range []string{oracleRequiredAttackerPassToScenario, oracleRequiredAttackerEmptyAttackScenario} {
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatal(err)
		}
		tr := strings.Join(res.Transcript, "\n")
		if len(res.Fails) != 0 {
			t.Fatalf("scenario failed: %v\n%s", res.Fails, tr)
		}
		if !strings.Contains(tr, "required attackers added (CR 508.1d)") {
			t.Fatalf("no forced-attacker note in the transcript:\n%s", tr)
		}
	}
}
