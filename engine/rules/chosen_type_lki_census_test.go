package rules

import (
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// chosenLKIReadCensus counts every corpus activated ability whose target
// filter (ValidTgts$) reads the SOURCE's recorded choice (ChosenType,
// IsNotChosenType, ChosenColor), by whether the ability's cost sacrifices the
// source itself. The sacrificing population is the one whose source has
// always left the battlefield before resolution, so its re-judgement must read
// the last-known choice (CR 113.7a/608.2h; SpecContext.SourceLKIValid,
// source_char_lki.go). Counts are exact in both directions: a new corpus shape
// must be given a test before the number moves.
var chosenLKIReadCensus = map[string]int{
	"sacrifices-self": 1, // Dawn-Blessed Pennant
	"other":           7,
}

func TestChosenLKITargetFilterCorpusCensus(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	got := map[string]int{}
	var sacrificers []string
	for _, c := range reg.AllCards() {
		for _, f := range c.Faces {
			for _, ab := range f.Abilities {
				tgts := ab.Params["ValidTgts"]
				if !strings.Contains(tgts, "ChosenType") && !strings.Contains(tgts, "ChosenColor") {
					continue
				}
				cost := ab.Params["Cost"]
				if strings.Contains(cost, "Sac<1/CARDNAME>") || strings.Contains(cost, "Sac<1/Self>") {
					got["sacrifices-self"]++
					sacrificers = append(sacrificers, f.Name)
				} else {
					got["other"]++
				}
			}
		}
	}
	sort.Strings(sacrificers)
	for k, want := range chosenLKIReadCensus {
		if got[k] != want {
			t.Errorf("census %q = %d, want %d (sacrificers: %v)", k, got[k], want, sacrificers)
		}
	}
	if len(got) != len(chosenLKIReadCensus) {
		t.Errorf("unexpected census classes: %v", got)
	}
	if len(sacrificers) == 0 {
		t.Fatal("census saw no sacrificing source: corpus missing or Params keys changed")
	}
}
