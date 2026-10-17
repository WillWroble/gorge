package rules

import "testing"

// Setup-drive parity with XMage's ScenarioReplay (lb-setupparity).
//
// A back-face battlefield placement is a seating, not a transform: Sephiroth,
// One-Winged Angel's "as this creature transforms, you get an emblem"
// replacement (R:Event$ Transform) must not run for the FlipFace the harness
// emits to seat the card on its back face (XMage's addCard makes no emblem).
func TestSetupBackFacePlacementRunsNoTransformReplacement(t *testing.T) {
	res, _ := runFixtureScenario(t, `{"name":"sephblock","setup":{"p0":{"battlefield":["Sephiroth, Fabled SOLDIER"],"back_face":["Sephiroth, Fabled SOLDIER"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"attack","seat":1,"attackers":["p1:Grizzly Bears"],"defender":"p0"},{"op":"block","seat":0,"blocks":[["p0:Sephiroth, Fabled SOLDIER","p1:Grizzly Bears"]]},{"op":"pass_to","seat":0,"step":"main2","active":"p1"}]}`)
	if got := setupPermCount(res.Snapshots[0], "Sephiroth, One-Winged Angel"); got != 1 {
		t.Fatalf("precondition: Sephiroth not on its back face at setup: %+v", res.Snapshots[0].Permanents)
	}
	last := res.Snapshots[len(res.Snapshots)-1]
	// Precondition: a creature really died in combat, so the emblem's
	// "whenever a creature dies" trigger would have fired had it existed.
	bearsDied := false
	for _, c := range last.Players[1].Graveyard {
		bearsDied = bearsDied || c == "Grizzly Bears"
	}
	if !bearsDied {
		t.Fatalf("precondition: Grizzly Bears did not die: p1 graveyard %v", last.Players[1].Graveyard)
	}
	for p, life := range []int32{last.Players[0].Life, last.Players[1].Life} {
		if life != 20 {
			t.Errorf("p%d life = %d, want 20 (no Super Nova emblem drain after a setup back-face placement)", p, life)
		}
	}
}

// XMage applies a setup "speed" inside the "setup" checkpoint at the first
// main phase (ScenarioReplay.applySetupState), after turn 1's upkeep. A
// static-GRANTED "Max speed — at the beginning of your upkeep" trigger
// (Endrider Spikespitter) therefore never fires in setup: the exile stays
// empty while speed still reads 4 at the checkpoint.
func TestSetupSpeedLandsAtFirstMainNotBeforeUpkeep(t *testing.T) {
	res, run := runFixtureScenario(t, `{"name":"speedsetup","setup":{"p0":{"battlefield":["Endrider Spikespitter"],"speed":4},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[]}`)
	snap := res.Snapshots[0]
	if snap.Checkpoint != "setup" {
		t.Fatalf("precondition: checkpoint = %s, want setup", snap.Checkpoint)
	}
	if got := run.e.G.Players[0].Speed; got != 4 {
		t.Fatalf("precondition: p0 speed = %d at the setup checkpoint, want 4", got)
	}
	if setupPermCount(snap, "Endrider Spikespitter") != 1 {
		t.Fatalf("precondition: Endrider Spikespitter not on the battlefield: %+v", snap.Permanents)
	}
	if ex := snap.Players[0].Exile; len(ex) != 0 {
		t.Errorf("p0 exile = %v at setup, want empty (the granted upkeep trigger fired during the setup drive)", ex)
	}
}
