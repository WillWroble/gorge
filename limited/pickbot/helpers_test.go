package pickbot_test

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/limited"
	pb "github.com/adams-shaun/gorge/limited/pickbot"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// corpus loads the shared card corpus once for the package's tests. A
// checkout with no .cards/ makes this skip — which reads green and tests
// nothing — so every corpus-backed assertion below lives behind it.
func corpus(t *testing.T) *cards.Registry {
	t.Helper()
	return testutil.CorpusRegistry(t)
}

// lookupOf wraps a registry into the pickbot Lookup shape.
func lookupOf(reg *cards.Registry) pb.CardLookup {
	return func(name string) *cards.Card {
		c, _ := reg.Lookup(name)
		return c
	}
}

// lookupCard resolves one name in the corpus or fails the test.
func lookupCard(t *testing.T, reg *cards.Registry, name string) *cards.Card {
	t.Helper()
	c, ok := reg.Lookup(name)
	if !ok || c == nil || len(c.Faces) == 0 {
		t.Fatalf("corpus has no card %q", name)
	}
	return c
}

// ref builds a CardRef with an id, name and rarity.
func ref(id, name string, rarity limited.Rarity) limited.CardRef {
	return limited.CardRef{ID: id, Name: name, Set: "FDN", Rarity: rarity}
}

// fdnMainSetNames lists every FDN sheet card's name from the embedded
// booster spec.
func fdnMainSetNames(t *testing.T) []string {
	t.Helper()
	spec, err := setdata.Load("FDN")
	if err != nil {
		t.Fatalf("setdata FDN: %v", err)
	}
	var names []string
	for sheet, cards := range spec.Sheets {
		if len(cards) == 0 {
			t.Fatalf("FDN sheet %q is empty", sheet)
		}
		for _, c := range cards {
			names = append(names, c.Name)
		}
	}
	return names
}
