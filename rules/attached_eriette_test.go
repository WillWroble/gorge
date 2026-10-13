package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestAttachedErietteGainsControl pins Eriette, the Beguiler's Attached
// line, whose bearer filter is TargetRelativeToSource$
// Permanent.nonLand+OppCtrl+cmcLEX with X the attaching Aura's mana value
// (CR 202.3): Pacifism (mana value 2) attaching to the opponent's Grizzly
// Bears (mana value 2) fires the trigger, and its GainControl LoseControl$
// UntilSourceUnattached binds that Aura, so p0 gains control of the Bears.
// Before, the matcher never read the parameter and the trigger stayed silent.
func TestAttachedErietteGainsControl(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	const sc = `{"name":"x","cr":["202.3"],"why":"x","setup":{"p0":{"battlefield":["Eriette, the Beguiler"],"hand":["Pacifism"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"cast","seat":0,"card":"p0:Pacifism","mana":"CW","targets":["p1:Grizzly Bears"]},{"op":"resolve","seat":0}]}`
	res, err := RunOracleScenarioJSON(reg, []byte(sc))
	if err != nil || len(res.Fails) != 0 {
		t.Fatalf("scenario: err=%v fails=%v", err, res.Fails)
	}
	for _, p := range res.Snapshots[len(res.Snapshots)-1].Permanents {
		if p.Name == "Grizzly Bears" {
			if p.Controller != 0 {
				t.Fatalf("Grizzly Bears controller %d, want 0 (Eriette's trigger)", p.Controller)
			}
			return
		}
	}
	t.Fatal("Grizzly Bears is not on the battlefield")
}
