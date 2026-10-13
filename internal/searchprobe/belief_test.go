package searchprobe

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

func beliefFixture(t *testing.T, names ...string) (*Belief, [][]*cards.Card) {
	t.Helper()
	reg := testutil.CorpusRegistry(t)
	var decks [][]*cards.Card
	for _, n := range names {
		d, err := testutil.LoadRepoDeck(reg, n)
		if err != nil {
			t.Fatal(err)
		}
		decks = append(decks, d)
	}
	b, err := NewBelief(decks)
	if err != nil {
		t.Fatal(err)
	}
	return b, decks
}

func deckNameSet(d []*cards.Card) map[string]bool {
	out := make(map[string]bool)
	for _, c := range d {
		out[c.Faces[0].Name] = true
	}
	return out
}

// A closed deal draws a list covering the seen colours, takes the seen cards
// out, trims to the unknown count, pads with the list's basics when it runs
// short, and is a pure function of its inputs and the seed.
func TestBeliefClosedDeal(t *testing.T) {
	b, decks := beliefFixture(t, "mono-red-prowess", "mono-blue-tempo", "mono-green-stompy")
	if b.Decks() != 3 {
		t.Fatalf("%d decks", b.Decks())
	}
	names := b.Cards().Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatal("the card table is not sorted by name")
		}
	}
	red := deckNameSet(decks[0])
	var seenRed []int32
	var mask uint8
	for _, c := range decks[0][:5] {
		o, ok := b.Ord(c.Faces[0].Name)
		if !ok {
			t.Fatalf("%s missing from the table", c.Faces[0].Name)
		}
		seenRed = append(seenRed, o)
		mask |= beliefColours(c)
	}
	sortInt32(seenRed)
	if mask == 0 {
		t.Fatal("fixture: the seen cards carry no colour")
	}
	for seed := uint64(0); seed < 20; seed++ {
		got := b.closedDeal(mask, seenRed, 10, rand.New(rand.NewPCG(seed, 1)), nil)
		if len(got) != 10 {
			t.Fatalf("dealt %d, want 10", len(got))
		}
		for _, o := range got {
			if !red[b.Cards().Name(int(o))] {
				t.Fatalf("seed %d: a red-seen opponent was dealt %s", seed, b.Cards().Name(int(o)))
			}
		}
		again := b.closedDeal(mask, seenRed, 10, rand.New(rand.NewPCG(seed, 1)), nil)
		if !reflect.DeepEqual(got, again) {
			t.Fatal("a closed deal is not a function of its seed")
		}
	}
	// Every card seen: the list is exhausted, so the deal pads with basics.
	var all []int32
	for _, c := range decks[0] {
		o, _ := b.Ord(c.Faces[0].Name)
		all = append(all, o)
	}
	sortInt32(all)
	got := b.closedDeal(mask, all, 7, rand.New(rand.NewPCG(3, 3)), nil)
	if len(got) != 7 {
		t.Fatalf("padded deal has %d cards", len(got))
	}
	for _, o := range got {
		c := b.Cards().Card(int(o))
		if !c.Faces[0].IsBasic() || !red[c.Faces[0].Name] {
			t.Fatalf("padding dealt %s, want the red list's basics", c.Faces[0].Name)
		}
	}
	// No list covers all five colours: the deal falls back to every list.
	if got := b.closedDeal(31, nil, 40, rand.New(rand.NewPCG(9, 9)), nil); len(got) != 40 {
		t.Fatalf("fallback deal has %d cards", len(got))
	}
}

// Closed keeps only the actor's own list.
func TestPublicGameClosed(t *testing.T) {
	b, decks := beliefFixture(t, "mono-red-prowess", "mono-blue-tempo")
	g := PublicGame{Names: []string{"a", "b"}, Decks: decks}
	c := g.Closed(state.PlayerID(1), b)
	if c.Belief != b || c.Decks[0] != nil || len(c.Decks[1]) != len(decks[1]) || g.Decks[0] == nil {
		t.Fatalf("Closed: %+v", c)
	}
	if open := g.Closed(0, nil); open.Belief != nil || open.Decks[1] == nil {
		t.Fatal("a nil belief closed the game")
	}
}

func sortInt32(s []int32) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
