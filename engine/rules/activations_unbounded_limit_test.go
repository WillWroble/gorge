package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// Elvish Refueler: "During your turn, as long as you haven't activated an
// exhaust ability this turn, you may activate exhaust abilities as though
// they haven't been activated" (Forge: S:Mode$ Activations | ValidSA$
// Activated.Exhaust | MinLimit$ -1 | PlayerTurn$ You | CheckSVar$ X |
// SVarCompare$ LT1). Exhaust is "activate each exhaust ability only once"
// (CR 702.177a); the static lifts that once-per-game cap, so the card's own
// exhaust ability, used on turn 1, is offered again on p0's next turn -- but
// never twice in one turn, because the first activation that turn fails the
// gate.
func TestActivationsUnboundedExhaust(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	run := func(steps string) []string {
		t.Helper()
		sc := `{"name":"refueler","cr":["702.177a"],"why":"inline","setup":{"p0":{"battlefield":["Elvish Refueler"]},"p1":{"battlefield":["Hill Giant"]}},"steps":[` + steps + `]}`
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return res.Fails
	}
	const exhaust = `{"op":"activate","seat":0,"card":"p0:Elvish Refueler","mana":"CG","ability_index":0},{"op":"resolve"}`
	const nextTurn = `{"op":"pass_to","seat":0,"step":"upkeep","active":"p0"},{"op":"pass_to","seat":0,"step":"main1","active":"p0"}`
	if fails := run(exhaust + "," + nextTurn + "," + exhaust); len(fails) != 0 {
		t.Fatalf("the exhaust ability was not offered again on p0's next turn: %v", fails)
	}
	if fails := run(exhaust + "," + exhaust); len(fails) == 0 || !strings.Contains(strings.Join(fails, " "), "not offered") {
		t.Fatalf("a second exhaust activation in one turn was offered: %v", fails)
	}
}
