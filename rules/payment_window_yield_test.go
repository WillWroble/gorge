package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// Brigid, Doun's Mind: "{T}: Add X {G} or X {W}, where X is the number of
// other creatures you control" and "At the beginning of your first main
// phase, you may pay {W}. If you do, transform Brigid." With no other
// creature her mana ability adds nothing, so the triggered cost's payment
// window (paymentManaAskClass) must not offer to tap her: she cannot help pay
// {W}. Tapping her is a legal mana activation (CR 605.1a), but the window's
// first-option fallback used to tap her for nothing (the ECL Level B rows
// Brigid trigger#0.1/#1.0, gorge "3/2 tapped", XMage untapped).

func TestBrigidZeroYieldNotOfferedToPayHerTransform(t *testing.T) {
	testutil.CorpusRegistry(t)
	res, _ := runFixtureScenario(t, `{"name":"brigid-plains","setup":{"p0":{"battlefield":["Brigid, Clachan's Heart","Plains"],"back_face":["Brigid, Clachan's Heart"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"pass_to","seat":0,"decision":"priority"},{"op":"pass","seat":0},{"op":"pass","seat":1},{"op":"resolve","seat":0}]}`)
	for _, line := range res.Transcript {
		if strings.Contains(line, "Tap Brigid, Doun's Mind for mana") {
			t.Fatalf("payment window offered the zero-yield Brigid: %s", line)
		}
	}
	snap := res.Snapshots[len(res.Snapshots)-1]
	var brigid, plains *OracleSnapPerm
	for i := range snap.Permanents {
		switch snap.Permanents[i].Name {
		case "Brigid, Clachan's Heart":
			brigid = &snap.Permanents[i]
		case "Plains":
			plains = &snap.Permanents[i]
		}
	}
	if brigid == nil {
		t.Fatalf("Brigid did not transform paying {W} with Plains: %+v", snap.Permanents)
	}
	if brigid.Tapped {
		t.Fatal("Brigid was tapped to pay her own transform cost")
	}
	if plains == nil || !plains.Tapped {
		t.Fatal("Plains did not pay the {W}")
	}
}

func TestBrigidAloneGoesStraightToDecline(t *testing.T) {
	testutil.CorpusRegistry(t)
	res, _ := runFixtureScenario(t, `{"name":"brigid-alone","setup":{"p0":{"battlefield":["Brigid, Clachan's Heart"],"back_face":["Brigid, Clachan's Heart"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"resolve","seat":0}]}`)
	for _, line := range res.Transcript {
		if strings.Contains(line, "Tap Brigid, Doun's Mind for mana") {
			t.Fatalf("payment window offered the zero-yield Brigid: %s", line)
		}
	}
	snap := res.Snapshots[len(res.Snapshots)-1]
	for _, p := range snap.Permanents {
		if p.Name == "Brigid, Doun's Mind" && p.Tapped {
			t.Fatal("Brigid was tapped for nothing")
		}
	}
}
