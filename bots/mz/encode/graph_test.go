package encode

import "testing"

// TestHashesMatchTheFork pins Hash64, IndexFor and Graph.Hash to the mage
// fork's StateEncoder and FeatureGraph: the values are the fork's own code
// run on the same names and the same graph (see docs/mz/LOG.md, stage 3).
func TestHashesMatchTheFork(t *testing.T) {
	for _, c := range []struct {
		s      string
		h      uint64
		bucket int32
	}{
		{"", 10533539378639503505, 1614623469},
		{"Tapped", 5169253511341831455, 1358050940},
		{"PRECOMBAT_MAIN", 7212206926030539290, 148331029},
		{"LifeTotal@20", 16984143781392419964, 545937907},
		{"a fairly long feature name over eight bytes", 16485342863624812849, 1804590803},
	} {
		if h := Hash64(c.s); h != c.h || IndexFor(h) != c.bucket {
			t.Errorf("Hash64(%q) = %d bucket %d, the fork has %d bucket %d", c.s, h, IndexFor(h), c.h, c.bucket)
		}
	}
	g := NewGraph()
	node := func(typ, name string, k, parent Key, label string) {
		if g.Add(typ, k, parent, label) {
			g.Add(name, NameKey(name), k, "name")
		}
	}
	p, c, c2 := Key(1), Key(2), Key(3)
	node("PLAYER", "Alice", p, RootKey, "NONE")
	g.Add("Tapped", NameKey("Tapped"), p, "NONE")
	g.AddNumeric("LifeTotal", 20, p, "NONE")
	g.AddNumeric("Power", -1, p, "NONE")
	node("CARD", "Island", c, p, "NONE")
	node("CARD", "Island", c2, p, "NONE")
	g.Add("LAND", NameKey("LAND"), c, "static")
	g.Add("LAND", NameKey("LAND"), c, "NONE")
	g.Add("LAND", NameKey("LAND"), c2, "static")
	node("PERMANENT", "Island", c2, c, "attachment")
	if h := g.Hash(); h != 17656720163347143049 {
		t.Errorf("graph hash %d, the fork has 17656720163347143049", h)
	}
}
