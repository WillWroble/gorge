package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/rules/chars"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// A Room spell on the stack has only the cast door's characteristics (CR
// 709.5), so the snapshot labels it by that door's name, not by the setup
// ref's "Left // Right" (lb-roomlabel).
func TestOracleRoomSpellStackLabelIsCastDoor(t *testing.T) {
	res, _ := runFixtureScenario(t, `{"name":"room","setup":{"p0":{"hand":["Dazzling Theater // Prop Room"]},"p1":{}},"steps":[{"op":"cast","seat":0,"card":"p0:Dazzling Theater // Prop Room","mana":"CCCW"}]}`)
	var snap *OracleSnapshot
	for i := range res.Snapshots {
		if len(res.Snapshots[i].Stack) > 0 {
			snap = &res.Snapshots[i]
		}
	}
	if snap == nil {
		t.Fatal("no checkpoint has the Room spell on the stack")
	}
	if got := snap.Stack[0].Source; got != "p0:Dazzling Theater" {
		t.Errorf("stack source = %q, want p0:Dazzling Theater", got)
	}
}

// Census: every two-door Room in the corpus, cast as either door, is labelled
// by that door's own name under the setup ref's seat prefix and ordinal.
func TestOracleRoomSpellStackLabelCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	n := 0
	for _, c := range reg.AllCards() {
		if c == nil || len(c.Faces) != 2 {
			continue
		}
		for fi := 0; fi < 2; fi++ {
			o := &state.Object{Card: c, FaceIdx: uint8(fi), Zone: state.ZStack}
			if !chars.IsRoomCard(o) {
				break
			}
			want := "p1:" + c.Faces[fi].Name + "#2"
			if got := stackSourceRef("p1:"+c.Faces[0].Name+" // "+c.Faces[1].Name+"#2", o); got != want {
				t.Errorf("%s door %d: label %q, want %q", c.Faces[0].Name, fi, got, want)
			}
			if fi == 0 {
				n++
			}
		}
	}
	if n < 30 {
		t.Fatalf("only %d Room cards in the corpus; census is vacuous (corpus missing?)", n)
	}
}
