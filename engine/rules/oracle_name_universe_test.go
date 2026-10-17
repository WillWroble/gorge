package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestOracleNameAskHonoursNonlandFilter pins the oracle runner's
// scenario-scoped name universe (oracleNameUniverse). Skyseer's Chariot
// ("As this Vehicle enters, choose a nonland card name", ValidCards$
// Card.nonLand) placed by setup used to be answered with the library top --
// Wastes, a land and not a legal answer -- because without a universe the
// as-enters ask saw only public objects and none was placed yet. With the
// universe the ask ranges over the scenario's own nonland cards, so the
// runner's pick is a legal name XMage knows, and a scenario without a
// name-choice card gets no universe at all.
func TestOracleNameAskHonoursNonlandFilter(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	raw := []byte(`{"name":"chariot-setup-name","cr":["607.2"],"why":"t",
		"setup":{"p0":{"battlefield":["Skyseer's Chariot","Colossal Dreadmaw"]},"p1":{"battlefield":["Grizzly Bears"]}},
		"steps":[]}`)
	res, err := RunOracleScenarioJSON(reg, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("fails: %v\n%v", res.Fails, res.Transcript)
	}
	var found bool
	for _, d := range res.Decisions {
		if d.Step >= 0 || len(d.PickKinds) != 1 || d.PickKinds[0] != "name" {
			continue
		}
		found = true
		if d.Options != 3 || len(d.Picks) != 1 || d.Picks[0] != "Colossal Dreadmaw" {
			t.Fatalf("setup name ask offered %d names and picked %v, want the 3 nonland scenario cards and Colossal Dreadmaw", d.Options, d.Picks)
		}
	}
	if !found {
		t.Fatalf("no setup name decision recorded: %+v", res.Decisions)
	}

}

// TestOracleNameUniverseOnlyForNameChoiceScenarios: a scenario seating no
// card-name choice gets no universe, so it plays exactly as before.
func TestOracleNameUniverseOnlyForNameChoiceScenarios(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	bears := lookup(t, reg, "Grizzly Bears")
	if u := oracleNameUniverse([][]*cards.Card{{bears, bears}}, nil); u != nil {
		t.Fatalf("a name-free scenario got a %d-card universe", u.Len())
	}
	chariot := lookup(t, reg, "Skyseer's Chariot")
	u := oracleNameUniverse([][]*cards.Card{{chariot, bears, bears}}, nil)
	if u.Len() != 2 {
		t.Fatalf("universe holds %d cards, want the 2 distinct seated cards", u.Len())
	}
}
