package setdata_test

import (
	"testing"

	_ "github.com/adams-shaun/gorge/rules"

	"github.com/adams-shaun/gorge/effects"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// TestSupportEverySheetCard is the support gate for the embedded specs:
// every card on every sheet resolves in the card corpus and is fully
// supported by the engine, so a generated pack never names a card the
// rules core cannot play. It runs the same check cmd/cardfuzz uses to
// build its pools: reg.Unsupported(c, effects.Supported()) is empty.
//
// The test lives in this external test package so the effects import never
// reaches the pure setdata package.
func TestSupportEverySheetCard(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	if reg.Len() == 0 {
		t.Fatal("corpus registry is empty: the support gate checked nothing")
	}
	sup := effects.Supported()
	if len(sup) == 0 {
		t.Fatal("effects.Supported() is empty: the support gate would pass anything")
	}
	// rules' package init registers the non-API primitives (kw:*, trig:*,
	// stat:*, repl:*); without it Supported() knows only the effect APIs
	// and this gate would reject every keyword card.
	if !sup["kw:Flying"] {
		t.Fatal("effects.Supported() lacks kw:Flying: the rules package did not register the non-API primitives")
	}
	checked := 0
	for _, code := range setdata.Codes() {
		spec, err := setdata.Load(code)
		if err != nil {
			t.Fatalf("Load(%s): %v", code, err)
		}
		for sheet, cards := range spec.Sheets {
			for _, sc := range cards {
				card, ok := reg.Lookup(sc.Name)
				if !ok {
					t.Errorf("%s %s: %q does not resolve in the corpus", code, sheet, sc.Name)
					continue
				}
				if unsupported := reg.Unsupported(card, sup); len(unsupported) > 0 {
					t.Errorf("%s %s: %q is unsupported: %v", code, sheet, sc.Name, unsupported)
				}
				checked++
			}
		}
	}
	if checked != 271+265 {
		t.Errorf("checked %d sheet cards, want the 271 FDN + 265 BLB sheet cards", checked)
	}
}
