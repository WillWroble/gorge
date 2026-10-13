package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// Kíli the Resourceful: "As long as you have an enduring story, you may pay
// {0} rather than pay the equip cost of the first equip ability you activate
// each turn." The AlternativeCost static scopes an ACTIVATED ability
// (ValidSA$ Activated.Equip), which only the attach-cost rider route priced;
// CR 118.9 lets the activator pay the alternative instead. With an enduring
// story (Kíli, Sol Ring and Bonesplitter: three artifacts/legendaries, CR
// 702.175a) the equip is activated for no mana; without one the alternative
// is not offered; and after the first equip of the turn it is gone.
func TestStaticAlternativeCostForEquip(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	run := func(battlefield string, extra string) []string {
		t.Helper()
		sc := `{"name":"kili","cr":["118.9"],"why":"inline","setup":{"p0":{"battlefield":[` + battlefield + `]}},"steps":[` +
			`{"op":"activate","seat":0,"card":"p0:Bonesplitter","ability":"(alternate cost)","targets":["p0:Kíli the Resourceful"]},{"op":"resolve"}` + extra + `]}`
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return res.Fails
	}
	again := `,{"op":"pass_to","seat":0,"decision":"priority","expect":[{"offered":{"seat":0,"kind":"ability","card":"p0:Bonesplitter","label":"(alternate cost)"},"want":false}]}`
	if fails := run(`"Kíli the Resourceful","Sol Ring","Bonesplitter"`, again); len(fails) != 0 {
		t.Fatalf("with an enduring story the first equip for {0} failed, or a second was offered: %v", fails)
	}
	if fails := run(`"Kíli the Resourceful","Bonesplitter"`, ""); len(fails) == 0 || !strings.Contains(strings.Join(fails, " "), "not offered") {
		t.Fatalf("the {0} equip was offered without an enduring story: %v", fails)
	}
}
