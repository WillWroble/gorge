package host

import (
	"encoding/json"
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/state"
)

// TestStartingChoiceHostPendingHidesHand is the host boundary half of CR
// 103.5: while the CR 103.1 play/draw choice is pending, neither the seat's
// scoped view (GET view?seat=, Registry.ViewAtSeat at head) nor the pending
// decision (GET /pending, Registry.Pending) may carry a hand card's object id.
// It drives a real two-human table whose hosted game opens on the toss
// winner's winner-chooses ask, reads the true hand ids off the parked engine,
// and walks the served JSON for any of them.
func TestStartingChoiceHostPendingHidesHand(t *testing.T) {
	t.Parallel()
	r, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.AddTable(starterFixtureTable("t1")); err != nil {
		t.Fatal(err)
	}
	if err := r.Start("t1"); err != nil {
		t.Fatal(err)
	}

	d0 := waitPending(t, r, "t1", 1, 1)
	if d0.Kind != decision.KStartingPlayer {
		t.Fatalf("first pending = kind %v, want the CR 103.1 toss-choice ask", d0.Kind)
	}

	_, m, err := r.lookup("t1", 1)
	if err != nil {
		t.Fatal(err)
	}
	var handIDs []state.ObjID
	if err := m.locked(func() error {
		for p := range m.e.G.Players {
			handIDs = append(handIDs, m.e.G.Zone(state.ZHand, state.PlayerID(p))...)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(handIDs) == 0 {
		t.Fatalf("fixture precondition: no hand cards dealt")
	}

	// GET /pending for the asked seat: the decision the endpoint serves.
	pd, err := r.Pending("t1", 1, 1)
	if err != nil {
		t.Fatalf("Pending(t1, 1, 1): %v", err)
	}
	assertNoHandIDsSerialised(t, "pending decision", pd, handIDs)

	// GET view?seat= for both seats at the live head.
	for _, seat := range []state.PlayerID{0, 1} {
		assertNoHandIDsSerialised(t, "seat view", r.seatViewAtHead(t, "t1", 1, seat), handIDs)
	}
}

// assertNoHandIDsSerialised marshals v and walks the JSON for any object id
// equal to one of the hidden hand's ids: the wire is the contract.
func assertNoHandIDsSerialised(t *testing.T, what string, v any, handIDs []state.ObjID) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", what, err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("%s: unmarshal: %v", what, err)
	}
	want := make(map[float64]bool, len(handIDs))
	for _, id := range handIDs {
		want[float64(id)] = true
	}
	var leaked []float64
	var walk func(any)
	walk = func(x any) {
		switch n := x.(type) {
		case map[string]any:
			if id, ok := n["id"].(float64); ok && want[id] {
				leaked = append(leaked, id)
			}
			for _, vv := range n {
				walk(vv)
			}
		case []any:
			for _, vv := range n {
				walk(vv)
			}
		}
	}
	walk(doc)
	if len(leaked) > 0 {
		t.Errorf("%s serialised %d hidden hand object id(s): %v", what, len(leaked), leaked)
	}
}
