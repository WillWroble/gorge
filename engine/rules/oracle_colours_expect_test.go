package rules

// oracle_colours_expect_test.go pins the runner's `colours` card expectation
// (CR 105.2): a card's current colour set as WUBRG letters, "" = colourless.

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

func coloursScenario(expect string) string {
	return `{"name":"colours-expect","cr":["105.2"],"why":"inline","setup":{"p0":{"battlefield":["Grizzly Bears","Ornithopter"]}},"steps":[` +
		`{"op":"pass_to","seat":0,"decision":"priority","expect":[` + expect + `]}]}`
}

func TestOracleColoursExpect(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	run := func(expect string) []string {
		res, err := RunOracleScenarioJSON(reg, []byte(coloursScenario(expect)))
		if err != nil {
			t.Fatal(err)
		}
		return res.Fails
	}
	if f := run(`{"card":"p0:Grizzly Bears","colours":"G"},{"card":"p0:Ornithopter","colours":""}`); len(f) != 0 {
		t.Fatalf("correct colour expectations failed: %v", f)
	}
	f := run(`{"card":"p0:Grizzly Bears","colours":"R"}`)
	if len(f) != 1 || !strings.Contains(f[0], `colours "G", want "R"`) {
		t.Fatalf("wrong colour expectation: fails = %v, want one colours mismatch", f)
	}
	f = run(`{"card":"p0:Ornithopter","colours":"U"}`)
	if len(f) != 1 || !strings.Contains(f[0], "colours") {
		t.Fatalf("colourless card asserted blue: fails = %v", f)
	}
}
