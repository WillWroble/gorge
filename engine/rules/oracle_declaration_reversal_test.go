package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// A scripted declaration the engine refuses as a whole is an illegal action:
// the oracle harness reverses it to the empty declaration (CR 733.1) and the
// scenario plays on, as XMage's test harness drops an illegal blocker or
// attacker (gorge-compliance combat/CantAttackOrBlockAloneTest
// test-cant-block-alone). Mogg Flunkies can't attack or block alone
// (CR 506.5), so its lone block is refused and Elite Vanguard's 2 damage
// connects.
const oracleBlockAloneReversedScenario = `{"name":"block-alone-reversed","cr":["733.1","506.5","509.1a"],"why":"a lone Mogg Flunkies block is reversed; the attacker is unblocked",
 "setup":{"p0":{"battlefield":["Mogg Flunkies"]},"p1":{"battlefield":["Elite Vanguard"]}},
 "steps":[{"op":"pass_to","seat":0,"step":"upkeep","active":"p1"},
  {"op":"attack","seat":1,"attackers":["p1:Elite Vanguard"],"defender":"p0"},
  {"op":"block","seat":0,"blocks":[["p0:Mogg Flunkies","p1:Elite Vanguard"]]},
  {"op":"pass_to","seat":0,"step":"end","active":"p1"},{"op":"resolve","seat":0}],
 "expect":[{"life":{"p0":18}},{"card":"p0:Mogg Flunkies","zone":"battlefield","damage":0}]}`

// The attack half: a lone Mogg Flunkies attack is reversed to no attack.
const oracleAttackAloneReversedScenario = `{"name":"attack-alone-reversed","cr":["733.1","506.5","508.1a"],"why":"a lone Mogg Flunkies attack is reversed; nobody is attacked",
 "setup":{"p0":{"battlefield":["Mogg Flunkies"]}},
 "steps":[{"op":"attack","seat":0,"attackers":["p0:Mogg Flunkies"],"defender":"p1"},
  {"op":"pass_to","seat":0,"step":"end","active":"p0"},{"op":"resolve","seat":0}],
 "expect":[{"life":{"p1":20}},{"card":"p0:Mogg Flunkies","tapped":false}]}`

func TestOracleIllegalDeclarationIsReversed(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	for _, sc := range []string{oracleBlockAloneReversedScenario, oracleAttackAloneReversedScenario} {
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Fails) != 0 {
			t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
		}
		if !strings.Contains(strings.Join(res.Transcript, "\n"), "reversed to the empty declaration (CR 733.1)") {
			t.Fatalf("no reversal in the transcript:\n%s", strings.Join(res.Transcript, "\n"))
		}
	}
}
