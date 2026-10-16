package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestValidSAMayPlaySourceRiderFires pins the SA-level `ValidSA$
// Spell.MayPlaySource` spelling of the may-play cast provenance. Undead
// Sprinter ("You may cast Undead Sprinter from your graveyard if a non-Zombie
// creature died this turn. If you do, Undead Sprinter enters with a +1/+1
// counter on it") and Hundred-Battle Veteran ("You may cast this card from
// your graveyard. If you do, it enters with a finality counter on it") carry
// the rider as a Static$ True SpellCast line whose ValidSA$ names the
// provenance. The matcher used to evaluate the clause as a plain spec filter,
// where MayPlaySource is no property, so the rider failed closed and the
// creature entered with no counter (CR 614.1c: the "enters with" replacement
// the rider sets up never existed).
func TestValidSAMayPlaySourceRiderFires(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	for _, tc := range []struct{ name, scenario, counter string }{
		{"Undead Sprinter", `{"name":"x","cr":["614.1c"],"why":"x","setup":{"p0":{"battlefield":["Grizzly Bears"],"hand":["Shock"],"graveyard":["Undead Sprinter"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"pass_to","seat":0,"step":"main1","decision":"priority"},{"op":"cast","seat":0,"card":"p0:Shock","mana":"R","targets":["p0:Grizzly Bears"]},{"op":"resolve","seat":0},{"op":"cast","seat":0,"card":"p0:Undead Sprinter","mana":"BR"},{"op":"resolve","seat":0}]}`, "P1P1"},
		{"Hundred-Battle Veteran", `{"name":"x","cr":["614.1c"],"why":"x","setup":{"p0":{"graveyard":["Hundred-Battle Veteran"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"pass_to","seat":0,"step":"main1","decision":"priority"},{"op":"cast","seat":0,"card":"p0:Hundred-Battle Veteran","mana":"CCCB"},{"op":"resolve","seat":0}]}`, "FINALITY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := RunOracleScenarioJSON(reg, []byte(tc.scenario))
			if err != nil || len(res.Fails) != 0 {
				t.Fatalf("scenario: err=%v fails=%v", err, res.Fails)
			}
			last := res.Snapshots[len(res.Snapshots)-1]
			for _, p := range last.Permanents {
				if p.Name == tc.name {
					if p.Counters[tc.counter] != 1 {
						t.Fatalf("%s entered with counters %v, want one %s", tc.name, p.Counters, tc.counter)
					}
					return
				}
			}
			t.Fatalf("%s is not on the battlefield: %+v", tc.name, last.Permanents)
		})
	}
}
