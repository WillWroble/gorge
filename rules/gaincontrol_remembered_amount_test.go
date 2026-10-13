package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// falconDonate turns a face-down Coveted Falcon up with two Wild Dogs beside
// it, picks the permanents named by want (indices: 0 = Falcon, 1/2 = Dogs) in
// the chained GainControl's target ask, and returns how many cards seat 0
// drew plus the controllers of the three permanents.
func falconDonate(t *testing.T, want map[int]bool) (drew int, ctrl [3]state.PlayerID) {
	t.Helper()
	reg := testutil.CorpusRegistry(t)
	e := New(Config{Seed: 13, Names: []string{"a", "b"},
		Decks: [][]*cards.Card{mountainDeck(t, 40), mountainDeck(t, 40)}})
	src := e.G.AddObject(mustCorpusCard(t, reg, "Coveted Falcon"), 0)
	e.emit(events.Event{Kind: events.MoveZone, Obj: src.ID, From: state.ZLibrary, To: state.ZBattlefield,
		Counter: events.FaceDownEntryCounterFor("Artifact & Creature", 2, 2, true)})
	if !e.G.Obj(src.ID).FaceDown {
		t.Fatal("precondition: Falcon must be face down")
	}
	ids := []state.ObjID{src.ID}
	for i := 0; i < 2; i++ {
		o := e.G.AddObject(mustCorpusCard(t, reg, "Wild Dogs"), 0)
		e.emit(events.Event{Kind: events.MoveZone, Obj: o.ID, From: state.ZLibrary, To: state.ZBattlefield})
		ids = append(ids, o.ID)
	}
	for _, id := range ids {
		if o := e.G.Obj(id); o.Zone != state.ZBattlefield || o.Controller != 0 {
			t.Fatalf("precondition: %d must be on the battlefield under seat 0", id)
		}
	}
	before := countDraws(e, 0)
	e.emit(events.Event{Kind: events.TurnFaceUp, Obj: src.ID, Player: 0})
	e.putTriggersOnStack()
	if len(e.G.Stack) == 0 {
		t.Fatal("precondition: the TurnFaceUp trigger must be on the stack")
	}
	picked := 0
	for n := 0; n < 40 && len(e.G.Stack) > 0; n++ {
		d := e.Pending()
		var ch []int
		switch {
		case d.Kind == decision.KPriority:
			ch = []int{0}
		case d.Max == 3 && d.Min == 0: // the chained GainControl's targets
			for i, o := range d.Options {
				for k, id := range ids {
					if o.Obj == id && want[k] {
						ch = append(ch, i)
						picked++
					}
				}
			}
		default:
			for i := range d.Options {
				if len(ch) < d.Max {
					ch = append(ch, i)
				}
			}
		}
		if err := e.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: ch}); err != nil {
			t.Fatalf("submit: %v", err)
		}
	}
	if picked != len(want) {
		t.Fatalf("picked %d of %d wanted targets", picked, len(want))
	}
	for i, id := range ids {
		ctrl[i] = e.G.Obj(id).Controller
		if want[i] && ctrl[i] != 1 {
			t.Fatalf("permanent %d controller = %d, want 1", i, ctrl[i])
		}
	}
	return countDraws(e, 0) - before, ctrl
}

// TestCovetedFalconDrawsPerPermanentGained: "Draw a card for each one they
// gained control of this way" -- the Remembered$Amount must count the Falcon
// itself even though it is also the trigger's fire-time capture.
func TestCovetedFalconDrawsPerPermanentGained(t *testing.T) {
	t.Parallel()
	if drew, _ := falconDonate(t, map[int]bool{0: true}); drew != 1 {
		t.Fatalf("donating only the Falcon drew %d, want 1", drew)
	}
	if drew, _ := falconDonate(t, map[int]bool{0: true, 1: true, 2: true}); drew != 3 {
		t.Fatalf("donating the Falcon and two Dogs drew %d, want 3", drew)
	}
	if drew, _ := falconDonate(t, map[int]bool{1: true, 2: true}); drew != 2 {
		t.Fatalf("donating two Dogs drew %d, want 2", drew)
	}
}
