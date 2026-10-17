package effects

import (
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// redirectCensusUnadmitted is the exact, shrink-only set of corpus
// "card/ReplacementEffects$ SVar" pairs that are Effect-delivered Event$ Moved
// redirects whose ReplaceWith$ head is a ChangeZone of the replaced card to
// exile (or, from the stack, to hand) yet that replacementRedirectsToExile
// still refuses -- each keeps its loud Note. A pair that becomes admitted must
// be deleted from this table (the test fails on a stale entry), and a new
// refusal fails until it is listed with its reason.
var redirectCensusUnadmitted = map[string]string{}

// TestEffectDeliveredExileRedirectCensus walks every corpus DB$ Effect and
// classifies its Event$ Moved ReplacementEffects$ lines: the "exile it
// instead" / "put it into your hand instead" family (Lilah, Feather, Gandalf
// of the Secret Fire, Forger's Foundry, Soulfire Grand Master and the bare
// Rest-in-Peace-style bodies) must be admitted by the registration gate, so
// the resolving spell never silently goes to the graveyard.
func TestEffectDeliveredExileRedirectCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	admitted := 0
	got := map[string]bool{}
	for _, card := range reg.AllCards() {
		for _, face := range card.Faces {
			for _, line := range face.SVars {
				if !strings.Contains(line, "ReplacementEffects$") || replacementBodyAPI(line) != "Effect" {
					continue
				}
				rawNames := replacementBodyParams(line)["ReplacementEffects"]
				for _, name := range strings.FieldsFunc(rawNames, func(r rune) bool { return r == ',' || r == ' ' }) {
					event, params := parseReplacementLine(face.SVars, name)
					if event != "Moved" {
						continue
					}
					body := face.SVars[replacementLineWith(params)]
					if replacementBodyAPI(body) != "ChangeZone" {
						continue
					}
					bp := replacementBodyParams(body)
					dest := bp["Destination"]
					if dest != "Exile" && !(dest == "Hand" && bp["Origin"] == "Stack") {
						continue
					}
					if bp["Defined"] != "ReplacedCard" && bp["Defined"] != "Remembered" {
						continue
					}
					if replacementRedirectsToExile(params, body, face.SVars) {
						admitted++
						continue
					}
					got[face.Name+"/"+name] = true
				}
			}
		}
	}
	if admitted == 0 {
		t.Fatal("precondition: the census admitted no redirect at all; the corpus scan is vacuous")
	}
	var keys []string
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, ok := redirectCensusUnadmitted[k]; !ok {
			t.Errorf("new unadmitted exile/hand redirect %s (add it to redirectCensusUnadmitted with a reason, or admit it)", k)
		}
	}
	for k := range redirectCensusUnadmitted {
		if !got[k] {
			t.Errorf("stale entry %s: it is admitted now (or gone); delete it", k)
		}
	}
	t.Logf("admitted=%d unadmitted=%d", admitted, len(got))
}
