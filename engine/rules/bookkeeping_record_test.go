package rules

// bookkeeping_record_test.go pins the two engine halves the level-B
// bookkeeping-trigger template (lab/oracle/compliance/oraclegen/templates/
// trigger_bookkeeping.go) observes through a consumer:
//
//   - api:Play's RememberPlayed$ True records the played card on the source.
//     Planetarium of Wan Shi Tong's "You may cast that card without paying
//     its mana cost. Do this only once each turn." reads that record: its
//     trigger's PresentDefined$ Remembered gate and the Play's
//     ConditionDefined$ Remembered gate stop a second free cast the same
//     turn, until the TurnBegin cleanup clears it.
//   - DB$ Cleanup | ClearImprinted$ True releases the ImprintOnHost$ link.
//     Superior Foes of Spider-Man: "you may play that card until you exile
//     another card with THIS creature" -- after the creature is flickered it
//     is a new object (CR 400.7), so a card the returned creature exiles
//     does not end the first card's permission.

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

const planetariumTwoScries = `{"name":"planetarium-once-each-turn","cr":["603.2"],"why":"inline","setup":{"p0":{"battlefield":["Planetarium of Wan Shi Tong"],"hand":["Opt","Preordain"],` +
	`"library_top":["Grizzly Bears","Grizzly Bears","Grizzly Bears","Grizzly Bears","Grizzly Bears","Grizzly Bears","Grizzly Bears","Grizzly Bears"]}},"steps":[` +
	`{"op":"cast","seat":0,"card":"p0:Opt","mana":"U"},` +
	`{"op":"resolve","seat":0,"answers":[{"kind":"choose","pick":["Continue"]},{"kind":"modes","pick":["Play Grizzly Bears"]}]},` +
	`{"op":"cast","seat":0,"card":"p0:Preordain","mana":"U"},` +
	`{"op":"resolve","seat":0}]}`

func TestPlanetariumFreeCastOnlyOnceEachTurn(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(planetariumTwoScries))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
	bears := func(i int) int {
		n := 0
		for _, p := range res.Snapshots[i].Permanents {
			if p.Controller == 0 && p.Name == "Grizzly Bears" {
				n++
			}
		}
		return n
	}
	// Precondition: the first scry's free cast happened (RememberPlayed$ has
	// something to record).
	if got := bears(2); got != 1 {
		t.Fatalf("after the first scry p0 controls %d Grizzly Bears, want 1 (the free cast)", got)
	}
	if got := bears(len(res.Snapshots) - 1); got != 1 {
		t.Fatalf("after the second scry the same turn p0 controls %d Grizzly Bears, want 1: Do this only once each turn", got)
	}
	for _, d := range res.Decisions {
		if d.Step == 3 && (d.Resume == "play" || d.Resume == "look_ack") {
			t.Fatalf("the second scry the same turn looked at the top card again: %+v", d)
		}
	}
}

const superiorFoesFlicker = `{"name":"superior-foes-new-object","cr":["400.7"],"why":"inline","setup":{"p0":{"battlefield":["Superior Foes of Spider-Man"],"hand":["Angel's Mercy","Cloudshift","Air Elemental"],"library_top":["Forest","Mountain"]}},"steps":[` +
	`{"op":"cast","seat":0,"card":"p0:Angel's Mercy","mana":"CCWW"},{"op":"resolve","seat":0},` +
	`%FLICKER%` +
	`{"op":"cast","seat":0,"card":"p0:Air Elemental","mana":"CCCUU"},{"op":"resolve","seat":0},` +
	`{"op":"pass_to","seat":0,"decision":"priority","expect":[{"offered":{"seat":0,"kind":"play","card":"p0:Forest"},"want":%WANT%}]}]}`

func TestSuperiorFoesFlickerKeepsTheFirstPermission(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	for _, tc := range []struct {
		name, flicker, want string
	}{
		// Control: the same creature exiles another card, which ends the
		// first card's permission.
		{"same object", "", "false"},
		// CR 400.7: the flickered creature is a new object; the card it
		// exiles is not "another card exiled with" the old one.
		{"flickered", `{"op":"cast","seat":0,"card":"p0:Cloudshift","mana":"W","targets":["p0:Superior Foes of Spider-Man"]},{"op":"resolve","seat":0},`, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := strings.NewReplacer("%FLICKER%", tc.flicker, "%WANT%", tc.want).Replace(superiorFoesFlicker)
			res, err := RunOracleScenarioJSON(reg, []byte(sc))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Fails) != 0 {
				t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
			}
			last := res.Snapshots[len(res.Snapshots)-1]
			if got := strings.Join(last.Players[0].Exile, ","); got != "Forest,Mountain" {
				t.Fatalf("p0 exile = %q, want both dug cards (Forest,Mountain)", got)
			}
		})
	}
}
