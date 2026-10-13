package rules

// dig_exiledwith_static_test.go pins the S:-line spelling of exile provenance
// on a Dig exile: Maralen, Fae Ascendant names ExiledWithSource ONLY in its
// MayPlay static's Affected$; its ETB trigger exiles through `DB$ Dig |
// DestinationZone$ Exile`, whose SVar carries no mention. The engine must
// still record "exiled with Maralen" on that move, or no card it exiled ever
// matches the static's Card.ExiledWithSource filter.

import (
	"testing"

	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// maralenDigScenario casts Maralen with Savannah Lions and Grizzly Bears on
// top of p1's library and p1's own Raging Goblin already in exile (exiled by
// nothing), then resolves the spell and its ETB Dig.
const maralenDigScenario = `{"name":"maralen-fae-ascendant-dig-exiled-with","cr":["607.2a"],"why":"inline",` +
	`"setup":{"p0":{"hand":["Maralen, Fae Ascendant"]},"p1":{"library_top":["Savannah Lions","Grizzly Bears"],"exile":["Raging Goblin"]}},"steps":[` +
	`{"op":"cast","seat":0,"card":"p0:Maralen, Fae Ascendant","mana":"CCBGU"},` +
	`{"op":"resolve"},` +
	`{"op":"resolve"}]}`

func TestDigExileRecordsExiledWithForStaticSource(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc, err := decodeOracleScenario([]byte(maralenDigScenario))
	if err != nil {
		t.Fatal(err)
	}
	sc.xmageFixture = true
	fails, transcript, run := runOracleScenarioSpare(reg, sc, false, new(Spare))
	if len(fails) != 0 {
		t.Fatalf("scenario failed: %v\n%v", fails, transcript)
	}
	e := run.e
	var maralen state.ObjID
	for _, id := range e.G.Zone(state.ZBattlefield, 0) {
		if o := e.G.Obj(id); o != nil && o.Face().Name == "Maralen, Fae Ascendant" {
			maralen = id
		}
	}
	if maralen == 0 {
		t.Fatalf("precondition: Maralen is not on the battlefield\n%v", transcript)
	}
	byName := map[string]*state.Object{}
	for _, id := range e.G.Zone(state.ZExile, 1) {
		o := e.G.Obj(id)
		byName[o.Face().Name] = o
	}
	for _, n := range []string{"Savannah Lions", "Grizzly Bears", "Raging Goblin"} {
		if byName[n] == nil {
			t.Fatalf("precondition: %s not in p1's exile (the ETB Dig did not run?): %v\n%v", n, byName, transcript)
		}
	}
	for _, n := range []string{"Savannah Lions", "Grizzly Bears"} {
		o := byName[n]
		if o.ExiledWith != maralen {
			t.Errorf("%s ExiledWith = %d, want Maralen %d", n, o.ExiledWith, maralen)
		}
		if !effects.MatchesSpecFrom(e.G, "Card.ExiledWithSource", o.ID, 0, maralen) {
			t.Errorf("%s does not match Card.ExiledWithSource from Maralen", n)
		}
	}
	// The provenance is per source, not "any exiled card".
	goblin := byName["Raging Goblin"]
	if goblin.ExiledWith != 0 || effects.MatchesSpecFrom(e.G, "Card.ExiledWithSource", goblin.ID, 0, maralen) {
		t.Errorf("the setup-exiled Raging Goblin reads as exiled with Maralen (ExiledWith %d)", goblin.ExiledWith)
	}
}
