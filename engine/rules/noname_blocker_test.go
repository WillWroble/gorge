package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// namelessBlockerScenario: p1 casts a Disguise card face down (CR 702.168a),
// then p0 attacks with Pompous Gadabout ("can't be blocked by creatures that
// don't have a name", ValidBlocker$ Creature.noName) and Giant Spider. A
// face-down permanent has no name (CR 708.2a), so at p1's declare-blockers
// decision (CR 509.1b) it may block the Spider but not Gadabout, while the
// named Grizzly Bears may block Gadabout.
const namelessBlockerScenario = `{"name":"nameless-blocker","cr":["509.1b","708.2a"],"why":"a face-down creature has no name, so it cannot block Pompous Gadabout","setup":{"p0":{"battlefield":["Pompous Gadabout","Giant Spider"]},"p1":{"battlefield":["Grizzly Bears"],"hand":["Bolrac-Clan Basher"]}},"steps":[{"op":"pass_to","seat":1,"step":"main1","active":"p1"},{"op":"cast","seat":1,"card":"p1:Bolrac-Clan Basher","mana":"CCC","cast_mode":"disguised"},{"op":"resolve","seat":1},{"op":"attack","seat":0,"attackers":["p0:Pompous Gadabout","p0:Giant Spider"],"defender":"p1"},{"op":"pass_to","seat":0,"decision":"blockers","expect":[{"can_block":{"blocker":"p1:Bolrac-Clan Basher","attacker":"p0:Giant Spider"},"want":true},{"can_block":{"blocker":"p1:Bolrac-Clan Basher","attacker":"p0:Pompous Gadabout"},"want":false},{"can_block":{"blocker":"p1:Grizzly Bears","attacker":"p0:Pompous Gadabout"},"want":true}]}]}`

func TestCantBlockByNoNameExcludesFaceDownBlocker(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(namelessBlockerScenario))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("CR 708.2a/509.1b: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
	// Precondition: the blocker really is face down at the checkpoint, so the
	// restriction is read off a nameless permanent and not a missing one.
	last := res.Snapshots[len(res.Snapshots)-1]
	faceDown := 0
	for _, p := range last.Permanents {
		if p.FaceDown && p.Controller == 1 {
			faceDown++
		}
	}
	if faceDown != 1 {
		t.Fatalf("precondition: p1 controls %d face-down permanents at the blockers checkpoint, want 1", faceDown)
	}
}
