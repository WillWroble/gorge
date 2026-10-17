package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// Altered Ego (B18): "You may have Altered Ego enter as a copy of any
// creature on the battlefield, except it enters with X additional +1/+1
// counters on it." Its DB$ Clone body chains SubAbility$ DBAddCounter (an
// ETB$ True PutCounter Defined$ Self), the rider effects.CloneETBSubChainOK
// admits into the as-enters copy election. The rider is part of the copy
// (CR 707.9b exception), so declining the optional replacement (CR 614.1c)
// enters the 0/0 as itself with no counters, and it dies to SBA.

func alteredEgoScenario(copyPick string, expect string) string {
	return `{"name":"altered-ego","cr":["707.9","614.1c"],"why":"B18","setup":{` +
		`"p0":{"hand":["Altered Ego"]},"p1":{"battlefield":["Silvercoat Lion"]}},` +
		`"steps":[{"op":"cast","seat":0,"card":"p0:Altered Ego","mana":"CCCCCGU","answers":[` +
		`{"kind":"choose","pick":["3"]}]},` +
		`{"op":"resolve","seat":0,"answers":[{"kind":"choose","pick":["` + copyPick + `"]}]}],"expect":[` + expect + `]}`
}

func runAlteredEgo(t *testing.T, raw string) []string {
	t.Helper()
	reg := testutil.CorpusRegistry(t)
	sc, err := decodeOracleScenario([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	fails, transcript, _ := runOracleScenario(reg, sc)
	joined := strings.Join(transcript, "\n")
	// Precondition: the copy election was actually posed and answered. A
	// withheld election leaves the second scripted answer unconsumed (a
	// fail), and the transcript records the copy answer by its option label.
	if strings.Count(joined, "[answer] p0 choose") != 2 || strings.Contains(strings.Join(fails, "\n"), oracleUnconsumedMarker) {
		t.Fatalf("copy election was not posed:\n%s\n%s", strings.Join(fails, "\n"), joined)
	}
	if len(fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", fails, joined)
	}
	return transcript
}

// TestAlteredEgoCopiesWithXCounters: accepting the election copies the Lion
// and the rider puts X=3 +1/+1 counters on the copy (2/2 + 3 = 5/5).
func TestAlteredEgoCopiesWithXCounters(t *testing.T) {
	t.Parallel()
	tr := runAlteredEgo(t, alteredEgoScenario("p1:Silvercoat Lion",
		`{"count":{"seat":0,"zone":"battlefield","name":"Silvercoat Lion"},"eq":1},`+
			`{"card":"p0:Silvercoat Lion","pt":"5/5","counters":{"P1P1":3}},`+
			`{"card":"p1:Silvercoat Lion","pt":"2/2"}`))
	if !strings.Contains(strings.Join(tr, "\n"), "Silvercoat Lion") {
		t.Fatalf("transcript never names the copy template:\n%s", strings.Join(tr, "\n"))
	}
}

// TestAlteredEgoDeclineEntersAsItselfWithoutCounters: declining with a
// creature available enters Altered Ego as a 0/0 with no rider counters, so
// it goes to the graveyard; nothing becomes a copy.
func TestAlteredEgoDeclineEntersAsItselfWithoutCounters(t *testing.T) {
	t.Parallel()
	runAlteredEgo(t, alteredEgoScenario("Enter as itself",
		`{"count":{"seat":0,"zone":"battlefield","name":"Altered Ego"},"eq":0},`+
			`{"count":{"seat":0,"zone":"graveyard","name":"Altered Ego"},"eq":1},`+
			`{"count":{"seat":0,"zone":"battlefield","name":"Silvercoat Lion"},"eq":0}`))
}
