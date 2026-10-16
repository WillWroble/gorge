package cards_test

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestStationPushBodyShape pins the Station ability's resolving body (CR
// 702.184a: "Put a number of charge counters on this permanent equal to the
// tapped creature's power"): a CHARGE PutCounter on Self, sized by X, tagged
// Keyword$ Station (the ValidSA$ Activated.Station scope), and with NO Cost$.
// Because it has no cost, it must stay out of GrantedKeywordAbility, which
// also feeds the granted-keyword offer loops: there it would be a free
// activation.
func TestStationPushBodyShape(t *testing.T) {
	t.Parallel()
	if cards.GrantedKeywordAbility("Station:8") != nil {
		t.Fatal("GrantedKeywordAbility synthesized a Station body: the offer loops would offer it without its tap cost")
	}
	sa := cards.KeywordPushAbility("Station:8")
	if sa == nil {
		t.Fatal("KeywordPushAbility(Station:8) = nil")
	}
	if sa.API != "PutCounter" || sa.Params["CounterType"] != "CHARGE" || sa.Params["CounterNum"] != "X" ||
		sa.Params["Defined"] != "Self" || sa.Params["Keyword"] != "Station" || sa.Params["KeywordLine"] != "Station:8" {
		t.Fatalf("Station body = %s %v", sa.API, sa.Params)
	}
	if _, ok := sa.Params["Cost"]; ok {
		t.Fatalf("Station body carries a Cost$ (%q): the tap is paid rules-side before the push", sa.Params["Cost"])
	}
	// The granted heads still route through the same reader.
	if cards.KeywordPushAbility("Crew:1") == nil || cards.KeywordPushAbility("Flying") != nil {
		t.Fatal("KeywordPushAbility must defer to GrantedKeywordAbility for every other head")
	}
}

// TestEveryStationCarrierSynthesizesItsPushBody is the corpus census for the
// Station class: every face printing a K:Station line must mint its stack
// body from that line, since the activation pushes exactly that line as the
// KeywordAbilityPush Counter (engine/rules/station.go). A carrier whose line does not
// synthesize would activate, tap its payer and put nothing on the stack.
func TestEveryStationCarrierSynthesizesItsPushBody(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	carriers := 0
	for _, c := range reg.AllCards() {
		if c == nil {
			continue
		}
		for _, f := range c.Faces {
			if f == nil {
				continue
			}
			param, ok := f.KeywordParam("Station")
			if !ok {
				continue
			}
			carriers++
			line := "Station"
			if param != "" {
				line += ":" + param
			}
			if sa := cards.KeywordPushAbility(line); sa == nil || !strings.EqualFold(sa.Params["Keyword"], "Station") {
				t.Errorf("%s: K:%s does not synthesize a Station push body", f.Name, line)
			}
		}
	}
	// 35 corpus carriers at the FORGE_REF this test was written against; a
	// census that finds none is reading an empty corpus, not passing.
	if carriers == 0 {
		t.Fatal("no Station carriers found: the corpus census read nothing")
	}
}
