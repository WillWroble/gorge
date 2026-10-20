package decks

import (
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestAllDecksResolve pins that every deck from the mage fork resolves
// against the corpus, card for card.
func TestAllDecksResolve(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	names := Names()
	if len(names) != 48 {
		t.Fatalf("got %d decks, want the fork's 48", len(names))
	}
	for _, n := range names {
		f, err := File(n)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		for _, c := range f.Cards {
			want += c.Count
		}
		cs, err := Load(reg, n)
		if err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		if len(cs) != want {
			t.Errorf("%s: resolved %d cards, want %d", n, len(cs), want)
		}
	}
}
