package templates_test

import (
	"testing"

	"github.com/adams-shaun/gorge/compliance/levelb"
	"github.com/adams-shaun/gorge/compliance/oraclegen/templates"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestStaticG7BackFaceGraveyardOfferServed: Emet-Selch, Unsundered
// static#1.0 (Hades, Sorcerer of Eld's "During your turn, you may play cards
// from your graveyard") was the G7 named gap "graveyard probe is exiled by
// the card's own Moved replacement when setup places it". The measured cause
// was the face, not the replacement: a fixture's setup graveyard placement
// does not run replacements, but the offer observation placed the card on
// its FRONT face, where the permission does not exist. The back-face retry
// (offerTry.backFace) serves it with the probe in the graveyard.
//
// (The Lunar Whale static#0.1 left this slice earlier, at lb-setupparity:
// static_offer_self_attack_test.go.)
func TestStaticG7BackFaceGraveyardOfferServed(t *testing.T) {
	const name, key = "Emet-Selch, Unsundered", "static#1.0"
	reg := testutil.CorpusRegistry(t)
	c, ok := reg.Lookup(name)
	if !ok {
		t.Fatalf("%s not in the corpus", name)
	}
	for _, r := range levelb.Requirements(c) {
		if r.Key != key {
			continue
		}
		it, skip := templates.GenerateB(reg, name, r)
		if skip != nil {
			t.Fatalf("%s %s skipped: %s", name, key, skip.Reason)
		}
		p0 := it.Scenario.Setup["p0"]
		if len(p0.BackFace) != 1 || p0.BackFace[0] != name || len(p0.Graveyard) == 0 {
			t.Fatalf("setup p0 = %+v, want %s on its back face and a graveyard probe", p0, name)
		}
		return
	}
	t.Fatalf("precondition: %s carries no requirement %s", name, key)
}
