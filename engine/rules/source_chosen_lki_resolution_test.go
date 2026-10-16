package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// A Killer Among Us: "Sacrifice A Killer Among Us, Reveal the chosen creature
// type: If target attacking creature token is the chosen type, put three +1/+1
// counters on it and it gains deathtouch until end of turn." The enchantment
// is sacrificed as the cost, so at resolution its live object has lost the
// "as it entered" choice; CR 608.2h says the ability uses the source's last
// known information, so the chosen Human still matches the Human token
// (Defined$ Targeted.ChosenType). Before the resolving Ctx carried the source
// LKI, the predicate read the cleared live object and nothing happened.
func TestSourceChosenTypeLKIAtResolution(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc := `{"name":"killer","cr":["608.2h"],"why":"inline","setup":{"p0":{"hand":["A Killer Among Us"]},"p1":{"battlefield":["Hill Giant"]}},"steps":[` +
		`{"op":"cast","seat":0,"card":"p0:A Killer Among Us","mana":"CCCCG"},{"op":"resolve","answers":[{"kind":"choose","pick":["Human"]}]},` +
		`{"op":"pass_to","seat":0,"step":"upkeep","active":"p0"},{"op":"pass_to","seat":0,"step":"main1","active":"p0"},` +
		`{"op":"attack","seat":0,"defender":"p1","attackers":["p0:token:Human Token"]},` +
		`{"op":"activate","seat":0,"card":"p0:A Killer Among Us","ability_index":0,"targets":["p0:token:Human Token"]},{"op":"resolve"}]}`
	res, err := RunOracleScenarioJSON(reg, []byte(sc))
	if err != nil || len(res.Fails) != 0 {
		t.Fatalf("run: err=%v fails=%v", err, res.Fails)
	}
	last := res.Snapshots[len(res.Snapshots)-1]
	p, ok := snapPerm(last, "p0:token:Human Token")
	if !ok || p.Counters["P1P1"] != 3 {
		t.Fatalf("the chosen-type Human token holds %v, want P1P1=3 (the sacrificed source's chosen type, CR 608.2h)", p.Counters)
	}
}
