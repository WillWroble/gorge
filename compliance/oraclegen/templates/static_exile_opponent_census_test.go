package templates_test

// static_exile_opponent_census_test.go is the corpus-wide census of the
// opponent-reachable exiled-with may-play class (ticket lb-mayplayopp): every
// Mode$ Continuous MayPlay$ True static whose Affected$ names ExiledWithSource
// WITHOUT a YouOwn qualifier, so the cards it covers can be an OPPONENT's
// (Azula, Cunning Usurper; Intellect Devourer; Nightveil Specter). Before the
// ticket the may-play spell walk visited only the caster's own cards, and
// the class's opponent-owned rows were skipped with a named engine gap.
//
// It is a ratchet: served may only rise and each skip reason may only fall.
// When a fixture or engine change serves another row, lower its skip and
// raise served here in the same commit; a regression fails.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/compliance/levelb"
	"github.com/adams-shaun/gorge/compliance/oraclegen/templates"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// wantOpponentExiledWithCensus is the measured split at lb-mayplayopp: 22
// rows. Azula, Cunning Usurper and Null Summoner -- the two rows that carried
// the "opponent-owned exiled card is not offered" gap -- are two of the three
// served.
var wantOpponentExiledWithCensus = map[string]int{
	"served": 3,
	"skip:static cards exiled by the source's Dig carry no exiled-with association (engine gap)": 10,
	"skip:static counts cards exiled with the source":                                            5,
	"skip:static look-at not observable":                                                         2,
	"skip:static no mana cost":                                                                   1,
	"skip:static a MayPlayAltManaCost$ card in exile is not offered (the engine delivers the alternative cost only on a hand cast)": 1,
}

// opponentExiledWithStatic reports whether st is the census class.
func opponentExiledWithStatic(st cards.Static) bool {
	if st.Mode != "Continuous" || strings.TrimSpace(st.ParamStr(cards.PKMayPlay)) != "True" {
		return false
	}
	affected := st.ParamStr(cards.PKAffected)
	return strings.Contains(affected, "ExiledWithSource") && !strings.Contains(affected, "YouOwn")
}

func TestOpponentExiledWithMayPlayCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	got := map[string]int{}
	for _, c := range reg.AllCards() {
		for _, r := range levelb.Requirements(c) {
			if r.Family != "static" || r.Face >= len(c.Faces) {
				continue
			}
			slot, err := strconv.Atoi(r.Slot)
			if err != nil || slot >= len(c.Faces[r.Face].Statics) || !opponentExiledWithStatic(c.Faces[r.Face].Statics[slot]) {
				continue
			}
			switch {
			case r.Gap != "":
				got["gap:"+r.Gap]++
			default:
				if _, skip := templates.GenerateB(reg, c.Faces[0].Name, r); skip != nil {
					got["skip:"+skip.Reason]++
				} else {
					got["served"]++
				}
			}
		}
	}
	if got["served"] == 0 {
		t.Fatalf("precondition: the census served no row (the class filter or the corpus is empty): %v", got)
	}
	if got["served"] < wantOpponentExiledWithCensus["served"] {
		t.Errorf("served fell: got %d, want >= %d", got["served"], wantOpponentExiledWithCensus["served"])
	}
	for k, n := range got {
		if k != "served" && n > wantOpponentExiledWithCensus[k] {
			t.Errorf("%s grew: got %d, want <= %d", k, n, wantOpponentExiledWithCensus[k])
		}
	}
}
