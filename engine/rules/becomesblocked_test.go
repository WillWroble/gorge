package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// api:BecomesBlocked (CR 509.1h): Trap Runner makes an unblocked attacker
// become blocked with no creature blocking it. Somberwald Alpha's "Whenever a
// creature you control becomes blocked" fires for it (CR 509.3c: an effect
// blocking the creature triggers "becomes blocked"). A blocked creature with
// no blocker deals no combat damage (Slither Blade, p1 stays at 20) unless it
// has trample, which assigns it all to the player (CR 702.19e: the pumped 7/5
// Yavimaya Wurm takes p1 to 13). The gorge-compliance
// combat/BecomesBlockedTest rows, re-authored here.
var becomesBlockedScenarios = []string{
	`{"name":"becomes-blocked-no-damage","cr":["509.1h","509.3c"],"why":"Trap Runner blocks Slither Blade by effect",
 "setup":{"p0":{"battlefield":["Slither Blade","Somberwald Alpha"]},"p1":{"battlefield":["Trap Runner"]}},
 "steps":[{"op":"attack","seat":0,"attackers":["p0:Slither Blade"],"defender":"p1"},
  {"op":"pass_to","seat":0,"step":"declare-blockers","active":"p0"},{"op":"pass","seat":0},
  {"op":"activate","seat":1,"card":"p1:Trap Runner","targets":["p0:Slither Blade"],"ability_index":0},
  {"op":"pass_to","seat":0,"step":"end","active":"p0"},{"op":"resolve","seat":0}],
 "expect":[{"life":{"p1":20}},{"card":"p0:Slither Blade","pt":"2/3"}]}`,
	`{"name":"becomes-blocked-trample","cr":["509.1h","509.3c","702.19e"],"why":"a trampler blocked by effect assigns all its damage to the player",
 "setup":{"p0":{"battlefield":["Yavimaya Wurm","Somberwald Alpha"]},"p1":{"battlefield":["Trap Runner","Forest"]}},
 "steps":[{"op":"attack","seat":0,"attackers":["p0:Yavimaya Wurm"],"defender":"p1"},
  {"op":"block","seat":1},{"op":"pass","seat":0},
  {"op":"activate","seat":1,"card":"p1:Trap Runner","targets":["p0:Yavimaya Wurm"],"ability_index":0},
  {"op":"pass_to","seat":0,"step":"end","active":"p0"},{"op":"resolve","seat":0}],
 "expect":[{"life":{"p1":13}},{"card":"p0:Yavimaya Wurm","pt":"7/5"}]}`,
}

func TestBecomesBlockedByEffect(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	for _, sc := range becomesBlockedScenarios {
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Fails) != 0 {
			t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
		}
	}
}
