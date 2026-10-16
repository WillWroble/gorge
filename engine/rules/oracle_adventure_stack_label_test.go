package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// An Adventure spell on the stack has only the Adventure face's
// characteristics (CR 715.3b), so the snapshot labels it by the Adventure's
// name, as XMage does, not by the card's (lb-xdefaults).
func TestOracleAdventureSpellStackLabelIsAdventureFace(t *testing.T) {
	res, _ := runFixtureScenario(t, `{"name":"adv","setup":{"p0":{"hand":["Bonecrusher Giant"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"cast","seat":0,"card":"p0:Stomp","mana":"RR","targets":["p1:Grizzly Bears"]}]}`)
	var snap *OracleSnapshot
	for i := range res.Snapshots {
		if len(res.Snapshots[i].Stack) > 0 {
			snap = &res.Snapshots[i]
		}
	}
	if snap == nil {
		t.Fatal("no checkpoint has the Adventure spell on the stack")
	}
	if got := snap.Stack[0].Source; got != "p0:Stomp" {
		t.Errorf("stack source = %q, want p0:Stomp", got)
	}
}

// Census: every Adventure card in the corpus, on the stack as its Adventure
// face, is labelled by that face's name under the setup ref's seat prefix and
// ordinal; cast as its creature/permanent front it keeps the setup ref.
func TestOracleAdventureSpellStackLabelCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	n := 0
	for _, c := range reg.AllCards() {
		if c == nil || c.AlternateMode != "Adventure" || len(c.Faces) != 2 {
			continue
		}
		ref := "p1:" + c.Faces[0].Name + "#2"
		adv := &state.Object{Card: c, FaceIdx: 1, Zone: state.ZStack}
		if adventureSpellFace(adv) == nil {
			continue
		}
		n++
		if got, want := stackSourceRef(ref, adv), "p1:"+c.Faces[1].Name+"#2"; got != want {
			t.Errorf("%s: adventure label %q, want %q", c.Faces[0].Name, got, want)
		}
		front := &state.Object{Card: c, FaceIdx: 0, Zone: state.ZStack}
		if got := stackSourceRef(ref, front); got != ref {
			t.Errorf("%s: front-face label %q, want %q", c.Faces[0].Name, got, ref)
		}
	}
	if n < 100 {
		t.Fatalf("only %d Adventure cards in the corpus; census is vacuous (corpus missing?)", n)
	}
}
