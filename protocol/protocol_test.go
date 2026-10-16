package protocol

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
	"github.com/adams-shaun/gorge/view"
)

var update = flag.Bool("update", false, "rewrite protocol/testdata goldens")

func seat(n uint8) *uint8 { return &n }

// nint is an *int literal for the draft-event fixtures.
func nint(n int) *int { return &n }

// fixtures is one frame per type with every field populated, so a golden
// pins the whole wire shape. Change a golden only with a protocol change
// (and a Version bump when it is not additive).
func fixtures(t *testing.T) []Frame {
	t.Helper()
	mk := func(ft FrameType, seq uint64, body any) Frame {
		f, err := NewFrame(ft, "t1", 7, seq, body)
		if err != nil {
			t.Fatal(err)
		}
		f.ID = 4182
		return f
	}
	v := view.View{Viewer: view.NoSeat, Visibility: "omniscient", Turn: 3, Step: "main1", Phase: "main1",
		Players: []view.PlayerView{{ID: 0, Name: "mono-red-goblins", Life: 20, Hand: []view.CardView{}, Battlefield: []view.CardView{
			{ID: 12, Name: "Goblin Guide", Types: "Creature Goblin Scout", ManaCost: "R", Power: 2, Toughness: 2,
				Printing: view.Printing{Name: "Goblin Guide"}, Token: "#12", Keywords: []string{"Haste"}}},
			Graveyard: []view.CardView{}, Exile: []view.CardView{}, Pool: map[string]int32{"R": 1}}},
		Stack: []view.StackView{{ID: 40, Kind: "trigger", Name: "Watcher", Text: "When Watcher enters, you gain 1 life.", Controller: 1, Source: 30,
			Targets: []view.TargetView{{Player: 0, IsPlayer: true, Label: "Select any target"}}}},
		// view.PendingView.Decider is *state.PlayerID, a distinct named type
		// from seat's *uint8; the explicit conversion is legal because both
		// pointer base types share the underlying type uint8 (Go spec,
		// Conversions: unnamed pointer types whose base types have identical
		// underlying types).
		Pending: []view.PendingView{{Source: 30, Controller: 1, Label: "Watcher: gain 1 life", Optional: true, Decider: (*state.PlayerID)(seat(1))}}}
	return []Frame{
		mk(THello, 0, Hello{Session: "s3", Tables: []TableInfo{{ID: "t1", Name: "Table 1", Seats: 4, Spectator: "omniscient", State: TableLive, Match: 7, Perpetual: true, Format: "commander", BotPolicy: "bot", Mulligans: 2}}}),
		mk(TWidget, 9130, Widget{Turn: 3, Step: "main1", Phase: "main1", Active: 0, Priority: 2, Life: []int32{20, 17, 12, 20}, Lost: []bool{false, false, false, false}, StackDepth: 1, Last: "Bob casts Bolt #2", State: MatchLive}),
		mk(TMatchStart, 0, MatchStart{Seats: []SeatInfo{{Name: "mono-red-goblins", Deck: "mono-red-goblins", Colour: SeatColours[0], Human: true, DeckID: "mono-red-goblins"}}, Seed: 12345, Spectator: "omniscient", BotPolicy: "bot"}),
		mk(TSnapshot, 9130, Snapshot{View: v, TurnStarts: []uint64{0, 402, 1180}, Head: 9130}),
		mk(TEvent, 9131, EventBody{Event: EventFrom(events.Event{Seq: 9131, Kind: events.MoveZone, Player: 1, Obj: 12, From: state.ZHand, To: state.ZBattlefield}), Line: "Goblin Guide #12 moves from hand to battlefield"}),
		mk(TDecision, 9131, DecisionBody{Player: 2, Kind: "priority", Prompt: "You have priority."}),
		mk(TMatchEnd, 9500, MatchEnd{Result: "win", Winner: seat(2), Head: "6c8f9e4512366476"}),
		mk(TTableHalted, 9500, TableHaltedBody{Reason: "intent rejected: choice 3 out of range (2 options)"}),
		mk(TTableState, 9500, TableStateBody{State: TableBetweenGames}),
		mk(TOverflow, 0, Overflow{Dropped: 17}),
		mk(TError, 0, ErrorBody{Code: "unknown_table", Message: "no table t9"}),
		mk(TRewind, 900, Snapshot{View: v, TurnStarts: []uint64{0, 402}, Head: 900, Seats: []SeatInfo{{Name: "mono-red-goblins", Deck: "mono-red-goblins", Colour: SeatColours[0], Human: true, DeckID: "mono-red-goblins"}}}),
		// Draft-event frames (E1): every field populated. Production sends
		// these with an empty table/match/seq; the mk harness shape is what
		// the golden pins.
		mk(TEventState, 0, EventState{Event: "ev-1", Set: "FDN", Phase: "ROUND1", Round: 1,
			Seats: []EventSeat{
				{Seat: 0, Kind: "human", Name: "Alice", Joined: true, Picks: 42, Ready: true},
				{Seat: 1, Kind: "bot", Policy: "az-aggro", Drafter: "rating-colour", Joined: true, Picks: 41},
				{Seat: 2, Kind: "bot", Policy: "bot", Drafter: "rating-colour", Joined: true, Picks: 40},
				{Seat: 3, Kind: "human", Joined: false, Picks: 0},
			}, You: nint(0),
			Series:    []EventSeries{{Round: 1, A: 0, B: 1, BestOf: 1, State: "live", Table: "ev1g1", Attempt: 2, Score: [2]int{0, 0}, Join: "/t/ev1g1?seat=0&token=abc"}},
			Standings: []EventStanding{{Seat: 0, Points: 6, MW: 1, OMW: 0.5, GW: 0.9, OGW: 0.45}}}),
		mk(TDraftPack, 0, DraftPack{Event: "ev-1", Seat: 1, Waiting: false, Round: 2, PickNo: 7,
			Pack: []DraftCard{{ID: "r2p7c3", Name: "Ajani Vengeant", Set: "FDN", Number: "201", Rarity: "R"}},
			Pool: []DraftCard{{ID: "r1p1c0", Name: "Blightsteel Colossus", Set: "FDN", Number: "250", Rarity: "M"}}}),
		mk(TPodProgress, 0, PodProgress{Event: "ev-1", Round: 3, Picks: []int{42, 42, 41, 40}, Queued: []int{1, 0, 1, 1}}),
		mk(TSeriesUpdate, 0, SeriesUpdate{Event: "ev-1", Series: EventSeries{Round: 2, A: 1, B: 3, BestOf: 1, State: "ended", Table: "ev1g4", Attempt: 1, Score: [2]int{1, 0}, Winner: nint(1)}}),
	}
}

func TestGoldens(t *testing.T) {
	for _, f := range fixtures(t) {
		got, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, '\n')
		path := filepath.Join("testdata", string(f.T)+".json")
		if *update {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v (run go test ./protocol/ -update to create it)", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from golden:\n%s\nwant:\n%s", f.T, got, want)
		}
	}
}

func TestFramesRoundTrip(t *testing.T) {
	for _, f := range fixtures(t) {
		raw, err := json.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		var back Frame
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatal(err)
		}
		if back.V != Version || back.T != f.T || back.ID != f.ID || back.Table != f.Table || back.Match != f.Match || back.Seq != f.Seq {
			t.Fatalf("%s: envelope changed in transit: %+v", f.T, back)
		}
		if !bytes.Equal(bytes.TrimSpace(back.Body), bytes.TrimSpace(f.Body)) {
			t.Fatalf("%s: body changed in transit", f.T)
		}
	}
}

func TestDecodeIntoTypedBody(t *testing.T) {
	f, err := NewFrame(TDecision, "t1", 1, 5, DecisionBody{Player: 3, Kind: "target", Prompt: "Pick"})
	if err != nil {
		t.Fatal(err)
	}
	var d DecisionBody
	if err := f.Decode(&d); err != nil || d.Player != 3 || d.Kind != "target" || d.Prompt != "Pick" {
		t.Fatalf("decoded %+v, %v", d, err)
	}
}

func TestEventFromNamesKindsZonesAndSteps(t *testing.T) {
	mv := EventFrom(events.Event{Seq: 1, Kind: events.MoveZone, Player: 2, Obj: 9, From: state.ZLibrary, To: state.ZHand, Secret: true})
	if mv.Kind != "move_zone" || mv.From != "library" || mv.To != "hand" || !mv.Secret || mv.Player != 2 || mv.Obj != 9 {
		t.Fatalf("%+v", mv)
	}
	st := EventFrom(events.Event{Kind: events.StepChange, Step: state.StepMain2})
	if st.Kind != "step" || st.Step != "main2" || st.From != "" || st.To != "" {
		t.Fatalf("%+v", st)
	}
	// A kind that carries no zone/step must not print the zero value's name.
	tp := EventFrom(events.Event{Kind: events.Tap, Obj: 4})
	if tp.From != "" || tp.To != "" || tp.Step != "" {
		t.Fatalf("Tap leaked zone/step zero values: %+v", tp)
	}
	bl := EventFrom(events.Event{Kind: events.DeclareBlockers, Pairs: [][2]state.ObjID{{3, 4}}})
	if len(bl.Pairs) != 1 || bl.Pairs[0] != [2]uint32{3, 4} {
		t.Fatalf("%+v", bl)
	}
	kw := EventFrom(events.Event{Kind: events.KeywordTriggerPush})
	if kw.Kind != "keyword_trigger_push" {
		t.Fatalf("keyword trigger kind on wire = %q", kw.Kind)
	}
	if EventFrom(events.Event{Kind: 250}).Kind != "unknown" {
		t.Fatal("unknown kind not named unknown")
	}
}

func TestSeatColoursCoverEightSeats(t *testing.T) {
	if len(SeatColours) < 8 {
		t.Fatalf("%d seat colours; the engine plays up to 8 seats", len(SeatColours))
	}
	seen := map[string]bool{}
	for _, c := range SeatColours {
		if seen[c] {
			t.Fatalf("duplicate colour %s", c)
		}
		seen[c] = true
	}
}
