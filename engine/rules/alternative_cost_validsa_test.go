package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// Leyline of Mutation: "You may pay {W}{U}{B}{R}{G} rather than pay the mana
// cost for spells that you cast" (CR 118.9). Its AlternativeCost static names
// no ValidCard$ (only ValidSA$ Spell | ValidPlayer$ You); the bare matcher
// read the absent spec as matching nothing, so the grant was never offered.
// With the Leyline out, a Grizzly Bears cast electing the alternative cost
// (the runner's cast_mode "alternative") is paid from exactly WUBRG; without
// it there is no alternative cast option at all.
func TestAlternativeCostGrantWithoutValidCard(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	run := func(battlefield string) []string {
		t.Helper()
		sc := `{"name":"leyline","cr":["118.9"],"why":"inline","setup":{"p0":{"battlefield":[` + battlefield + `],"hand":["Grizzly Bears"]}},"steps":[` +
			`{"op":"cast","seat":0,"card":"p0:Grizzly Bears","mana":"WUBRG","cast_mode":"alternative"},{"op":"resolve"}]}`
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if len(res.Fails) == 0 {
			if pool := res.Snapshots[len(res.Snapshots)-1].Players[0].Pool; pool != "" {
				t.Fatalf("the alternative cost left %q floating", pool)
			}
		}
		return res.Fails
	}
	if fails := run(`"Leyline of Mutation"`); len(fails) != 0 {
		t.Fatalf("Leyline of Mutation's alternative cost was not offered: %v", fails)
	}
	if fails := run(``); len(fails) == 0 || !strings.Contains(strings.Join(fails, " "), "not offered") {
		t.Fatalf("an alternative cast was offered with no AlternativeCost static: %v", fails)
	}
}

// A self-carried ValidCard$-less AlternativeCost (the Bringer cycle's
// "ValidSA$ Spell.Self | EffectZone$ All") is priced by the face walk only:
// the battlefield-static route skips a static whose source is the card being
// cast, so the one alternative is offered once, never twice.
func TestAlternativeCostSelfStaticOfferedOnce(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc := `{"name":"bringer","cr":["118.9"],"why":"inline","setup":{"p0":{"hand":["Bringer of the White Dawn"]}},"steps":[` +
		`{"op":"mana","seat":0,"mana":"WUBRG","expect":[` +
		`{"offered":{"seat":0,"kind":"cast","card":"p0:Bringer of the White Dawn","label":"(alternative cost)"},"want":true},` +
		`{"offered":{"seat":0,"kind":"cast","card":"p0:Bringer of the White Dawn","label":"alternative cost 2"},"want":false}]}]}`
	res, err := RunOracleScenarioJSON(reg, []byte(sc))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("Bringer's self alternative cost offer count is wrong: %v", res.Fails)
	}
}
