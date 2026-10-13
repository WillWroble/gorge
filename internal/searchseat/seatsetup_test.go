package searchseat

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/searchprobe"
	"github.com/adams-shaun/gorge/internal/testutil"
)

type beliefSeat struct{ b *searchprobe.Belief }

func (s beliefSeat) SearchBelief() *searchprobe.Belief { return s.b }

// SeatSetup closes the game for a ClosedListSeat with a belief and leaves it
// open for every other seat (and a closed-list seat whose belief is nil).
func TestSeatSetupClosesOnlyForAClosedListSeat(t *testing.T) {
	_, decks := testutil.SampleDecks(t, 3)
	b, err := searchprobe.NewBelief(decks[2:])
	if err != nil {
		t.Fatal(err)
	}
	g := searchprobe.PublicGame{Names: []string{"a", "b"}, Decks: [][]*cards.Card{decks[0], decks[1]}}
	c := SeatSetup(g, beliefSeat{b}, 1)
	if c.Belief != b || c.Decks[0] != nil || c.Decks[1] == nil {
		t.Fatal("a closed-list seat's game was not closed to its opponent")
	}
	for _, s := range []any{struct{}{}, beliefSeat{}} {
		if o := SeatSetup(g, s, 1); o.Belief != nil || o.Decks[0] == nil {
			t.Fatalf("%T: an open seat's game was closed", s)
		}
	}
}
