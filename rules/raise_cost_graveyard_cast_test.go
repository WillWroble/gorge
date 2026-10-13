package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// Aven Interrupter: "Spells your opponents cast from graveyards or from exile
// cost {2} more to cast." The RaiseCost static keys on Forge's caster-agnostic
// Card.wasCastFromGraveyard, whose effects-side CastFlags read is stamped only
// by the pay-time CastInfo -- after the cost is determined (CR 601.2f precedes
// 601.2h) -- so the raise never applied and a flashback cast paid its bare
// flashback cost. CR 601.2f: the total cost includes cost increases; the cast
// comes from the graveyard, so the {2} applies.
func TestRaiseCostOnGraveyardCastAppliesToFlashback(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	run := func(withAven bool) string {
		t.Helper()
		p1 := `"p1":{}`
		if withAven {
			p1 = `"p1":{"battlefield":["Aven Interrupter"]}`
		}
		sc := `{"name":"aven-raise","cr":["601.2f"],"why":"inline","setup":{"p0":{"graveyard":["Think Twice"]},` + p1 + `},"steps":[` +
			`{"op":"cast","seat":0,"card":"p0:Think Twice","mana":"CCCCU","cast_mode":"flashback"},{"op":"resolve"}]}`
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if len(res.Fails) != 0 {
			t.Fatalf("fails: %v", res.Fails)
		}
		last := res.Snapshots[len(res.Snapshots)-1]
		return last.Players[0].Pool
	}
	if pool := run(false); pool != "CC" {
		t.Fatalf("precondition: without the static the flashback cast leaves %q floating, want CC", pool)
	}
	if pool := run(true); pool != "" {
		t.Fatalf("with Aven Interrupter the flashback cast left %q floating: the {2} raise was not applied (CR 601.2f)", pool)
	}
}

// A spell cast from the HAND is not cast from a graveyard: the raise must not
// apply (the pre-push read is the object's current zone).
func TestRaiseCostOnGraveyardCastSkipsHandCast(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc := `{"name":"aven-hand","cr":["601.2f"],"why":"inline","setup":{"p0":{"hand":["Think Twice"]},"p1":{"battlefield":["Aven Interrupter"]}},"steps":[` +
		`{"op":"cast","seat":0,"card":"p0:Think Twice","mana":"CU"},{"op":"resolve"}]}`
	res, err := RunOracleScenarioJSON(reg, []byte(sc))
	if err != nil || len(res.Fails) != 0 {
		t.Fatalf("a hand cast of Think Twice for its printed {1}{U} failed under Aven Interrupter: err=%v fails=%v", err, res.Fails)
	}
}
