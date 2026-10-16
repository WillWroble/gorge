package rules

import "testing"

// Etrata, Deadly Fugitive: "Whenever an Assassin you control deals combat
// damage to an opponent, cloak the top card of that player's library."
// Forge spells the card as Defined$ ValidLibrary Card.TopLibrary+OwnedBy
// TriggeredTarget, which used to be a loud no-op (the measured D5a diverge:
// p1 library 39 in gorge, 38 in XMage). The selector ranges over every
// player's library (Forge's game.getCardsIn), and the cloaked card enters
// under the CLOAKING player's control (CR 110.2a), face down.
func TestEtrataCloaksTheDamagedOpponentsTopCard(t *testing.T) {
	res, _ := runFixtureScenario(t, `{"name":"etrata","setup":{"p0":{"battlefield":["Etrata, Deadly Fugitive","Avian Changeling"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"attack","seat":0,"attackers":["p0:Avian Changeling"],"defender":"p1"},{"op":"pass_to","seat":0,"step":"main2"}]}`)
	setup, last := res.Snapshots[0], res.Snapshots[len(res.Snapshots)-1]
	if got, want := last.Players[1].LibraryCount, setup.Players[1].LibraryCount-1; got != want {
		t.Fatalf("p1 library = %d after the trigger, want %d (one card cloaked)", got, want)
	}
	if got := last.Players[0].LibraryCount; got != setup.Players[0].LibraryCount {
		t.Errorf("p0 library = %d, want %d unchanged (the damaged player's library is cloaked)", got, setup.Players[0].LibraryCount)
	}
	faceDown := 0
	for _, p := range last.Permanents {
		if p.FaceDown {
			faceDown++
			if p.Controller != 0 || p.Owner != 1 {
				t.Errorf("cloaked permanent controller %d owner %d, want controller 0 owner 1", p.Controller, p.Owner)
			}
			if p.PT != "2/2" {
				t.Errorf("cloaked permanent P/T %q, want 2/2", p.PT)
			}
		}
	}
	if faceDown != 1 {
		t.Fatalf("face-down permanents = %d, want 1: %+v", faceDown, last.Permanents)
	}
}
