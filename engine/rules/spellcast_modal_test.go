package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// Riku of Many Paths: "Whenever you cast a modal spell, choose up to X, where
// X is the number of times you chose a mode for that spell". Healing Salve is
// modal (CR 700.2: one of two bulleted modes is chosen), so the cast fires
// Riku's trigger (Spell.Modal), and X is 1 (NumTimesChoseMode), so its own
// "create a 1/1 blue Bird" mode can be chosen. A non-modal spell (Shock)
// does not fire it.
func TestSpellCastModalFiresRiku(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	const modal = `{"name":"riku-modal","cr":["700.2","603.2"],"why":"a modal spell fires Riku with X = 1",
 "setup":{"p0":{"battlefield":["Riku of Many Paths"],"hand":["Healing Salve"]},"p1":{"battlefield":["Grizzly Bears"]}},
 "steps":[{"op":"cast","seat":0,"card":"p0:Healing Salve","mana":"W","targets":["p0"],
   "answers":[{"kind":"modes","pick":["Target player gains 3 life."]},{"kind":"modes","pick":["Create a 1/1 blue Bird creature token with flying."]}]},
  {"op":"resolve"},{"op":"resolve"}]}`
	res, err := RunOracleScenarioJSON(reg, []byte(modal))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario did not play cleanly (X must be 1 for the Bird mode to be offered): %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
	if !rikuOnStack(res.Snapshots) {
		t.Fatal("Riku's SpellCast trigger never reached the stack for a modal spell")
	}
	bird := false
	for _, p := range res.Snapshots[len(res.Snapshots)-1].Permanents {
		bird = bird || strings.Contains(strings.ToLower(p.Name), "bird")
	}
	if !bird {
		t.Fatalf("no Bird token after Riku's trigger resolved with its token mode; transcript:\n%s", strings.Join(res.Transcript, "\n"))
	}

	const plain = `{"name":"riku-plain","cr":["700.2"],"why":"a non-modal spell does not fire Riku",
 "setup":{"p0":{"battlefield":["Riku of Many Paths"],"hand":["Shock"]},"p1":{"battlefield":["Grizzly Bears"]}},
 "steps":[{"op":"cast","seat":0,"card":"p0:Shock","mana":"R","targets":["p1"]}]}`
	res, err = RunOracleScenarioJSON(reg, []byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	if rikuOnStack(res.Snapshots) {
		t.Fatal("Riku fired for a non-modal spell")
	}
}

func rikuOnStack(snaps []OracleSnapshot) bool {
	for _, s := range snaps {
		for _, e := range s.Stack {
			if e.Kind == "ability" && strings.Contains(strings.ToLower(e.Source), "riku") {
				return true
			}
		}
	}
	return false
}
