package cards

import "testing"

func TestPrimitivesReportUnimplementedAtEOT(t *testing.T) {
	has := func(src, want string) bool {
		c, _ := ParseBytes("t.txt", []byte(src))
		if c == nil {
			t.Fatal("no card")
		}
		for _, p := range c.Primitives() {
			if p == want {
				return true
			}
		}
		return false
	}
	card := func(v string) string {
		return "Name:T\nManaCost:1\nTypes:Sorcery\nA:SP$ Pump | Cost$ 1 | AtEOT$ " + v + " | NumAtt$ +1\nOracle:x\n"
	}
	if !has(card("Library"), "api:AtEOT.Library") {
		t.Error("AtEOT$ Library must surface as an unsupported primitive")
	}
	if has(card("ExileCombat"), "api:AtEOT.ExileCombat") {
		t.Error("AtEOT$ ExileCombat is implemented")
	}
}
