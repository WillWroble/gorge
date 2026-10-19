package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// Rowan Kenrith's +2 creates an Effect that remembers its target player
// (RememberObjects$ Targeted) and carries `Mode$ Phase | Phase$ Cleanup |
// ValidPlayer$ Player.IsRemembered | Execute$ ExileEffect`. The trigger's
// "remembered" is the EFFECT's own memory (CR 611.2a: the effect lasts for
// that player's next turn only), not Rowan's, whose DBCleanup clears its own
// list the moment the ability resolves. So the Effect is exiled at p1's
// cleanup and the Bears attack exactly once (gorge-compliance
// continuous/GideonJuraAndRowanKenrithNextTurnTest
// test_SingleOpponentMustAttackRowanKenrith).
const oracleRowanEffectCleanupScenario = `{"name":"rowan-effect-ends-at-cleanup","cr":["611.2a","508.1d"],"why":"Rowan's taunt lasts for the target player's next turn only",
 "setup":{"p0":{"battlefield":["Rowan Kenrith"]},"p1":{"battlefield":["Balduvian Bears"]}},
 "steps":[{"op":"resolve","seat":0},
  {"op":"activate","seat":0,"card":"p0:Rowan Kenrith","targets":["p1"],"ability_index":0},
  {"op":"pass_to","seat":0,"step":"main2","active":"p0","expect":[{"life":{"p0":20}}]},
  {"op":"pass_to","seat":0,"step":"upkeep","active":"p1"},
  {"op":"pass_to","seat":0,"step":"main2","active":"p1","expect":[{"life":{"p0":18}}]},
  {"op":"pass_to","seat":0,"step":"upkeep","active":"p0"},
  {"op":"pass_to","seat":0,"step":"main2","active":"p0","expect":[{"life":{"p0":18}}]},
  {"op":"pass_to","seat":0,"step":"upkeep","active":"p1"},
  {"op":"pass_to","seat":0,"step":"main2","active":"p1","expect":[{"life":{"p0":18}}]}],
 "expect":[{"life":{"p0":18}}]}`

func TestEffectPhaseTriggerReadsTheEffectsRememberedPlayer(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(oracleRowanEffectCleanupScenario))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
}
