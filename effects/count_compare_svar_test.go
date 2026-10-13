package effects

import (
	"regexp"
	"sort"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

var zoneCountSVarBound = regexp.MustCompile(`HasPropertyHasCardsIn(?:Hand|Graveyard)_[^_\s/]+(?:_[^_\s/]+)*_(GE|GT|LE|LT|EQ)([A-Za-z][A-Za-z0-9]*)`)

// TestZoneCountHasPropertySVarBoundCensus is the corpus ratchet for the
// mechanism: every HasPropertyHasCardsIn{Hand,Graveyard}_<spec>_<OP><name>
// whose bound is an SVar NAME (Wojek Investigator, Consuming Tide: GTY) must
// name an SVar printed on the same face and resolve through
// parseCountCompareOrSVar, and the carrier set is pinned so a new carrier is
// a conscious addition.
func TestZoneCountHasPropertySVarBoundCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	h := newHost(t, 2)
	var carriers []string
	for _, card := range reg.AllCards() {
		for _, f := range card.Faces {
			for _, body := range f.SVars {
				m := zoneCountSVarBound.FindStringSubmatch(body)
				if m == nil {
					continue
				}
				carriers = append(carriers, f.Name)
				if _, named := f.SVars[m[2]]; !named {
					t.Errorf("%s: bound SVar %q is not printed on the face", f.Name, m[2])
				}
				c := &Ctx{Controller: 0, SVars: f.SVars}
				if _, _, ok := parseCountCompareOrSVar(h, c, m[1]+m[2]); !ok {
					t.Errorf("%s: bound %s%s does not resolve", f.Name, m[1], m[2])
				}
			}
		}
	}
	sort.Strings(carriers)
	want := []string{"Consuming Tide", "Wojek Investigator"}
	if len(carriers) != len(want) || carriers[0] != want[0] || carriers[1] != want[1] {
		t.Fatalf("SVar-bound zone-count carriers = %v, want %v", carriers, want)
	}
}

// TestZoneCountHasPropertySVarBound pins the Wojek Investigator shape: the
// bound Y is the controller's hand ("more cards than you"), read once, and an
// unprinted bound name fails closed.
func TestZoneCountHasPropertySVarBound(t *testing.T) {
	h := newHost(t, 3)
	const expr = "Count$PlayerCountOpponents$HasPropertyHasCardsInHand_Card_GTY"
	c := &Ctx{Controller: 0, SVars: map[string]string{"Y": "Count$CardsInYourHand"}}
	pcsZoneObjs(t, h, 0, 2, pcsMountain, state.ZHand)
	pcsZoneObjs(t, h, 1, 3, pcsMountain, state.ZHand)
	pcsZoneObjs(t, h, 2, 2, pcsMountain, state.ZHand)
	if len(h.g.Zone(state.ZHand, 0)) != 2 || len(h.g.Zone(state.ZHand, 1)) != 3 {
		t.Fatal("precondition: hands 2/3/2 required")
	}
	if got, ok := EvalCountOK(h, c, expr); !ok || got != 1 {
		t.Fatalf("only the 3-card opponent exceeds the controller's 2 = (%d, %v), want (1, true)", got, ok)
	}
	pcsZoneObjs(t, h, 2, 2, pcsMountain, state.ZHand) // 4 cards
	if got, ok := EvalCountOK(h, c, expr); !ok || got != 2 {
		t.Fatalf("both opponents exceed = (%d, %v), want (2, true)", got, ok)
	}
	c.SVars = map[string]string{}
	if got, ok := EvalCountOK(h, c, expr); ok {
		t.Fatalf("unprinted bound SVar = (%d, %v), want fail closed", got, ok)
	}
}
