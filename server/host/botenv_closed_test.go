package host

import (
	"context"
	"testing"

	"github.com/adams-shaun/gorge/bots/heuristic"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/bots/searchprobe"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// closedSpy is a spy EnvSeat with a closed opponent list
// (searchseat.ClosedListSeat): it records the Setup each Env carried.
type closedSpy struct {
	*spyEnvSeat
	belief *searchprobe.Belief
	setups []searchprobe.PublicGame
}

func (s *closedSpy) SearchBelief() *searchprobe.Belief { return s.belief }

func (s *closedSpy) DecideEnv(ctx context.Context, env bots.Env, d decision.Decision) (decision.Intent, error) {
	s.setups = append(s.setups, env.Search.Setup)
	return s.spyEnvSeat.DecideEnv(ctx, env, d)
}

// A closed-list EnvSeat is handed a closed game: its Setup holds its own list
// and the belief but not the opponent's list, and its honest root deals the
// opponent's hidden cards from the belief (never the live engine's cards).
func TestHostedClosedListSeatGetsAClosedRoot(t *testing.T) {
	t.Parallel()
	r, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	const seed = uint64(20261013)
	if err := r.AddTable(envTestTable("t1", seed)); err != nil {
		t.Fatal(err)
	}
	r.mu.RLock()
	tbl := r.tables["t1"]
	r.mu.RUnlock()
	_, decks := testutil.SampleDecks(t, 4)
	b, err := searchprobe.NewBelief(decks[2:])
	if err != nil {
		t.Fatal(err)
	}
	inBelief := make(map[string]bool)
	for _, n := range b.Cards().Names() {
		inBelief[n] = true
	}
	spy := &closedSpy{spyEnvSeat: &spyEnvSeat{Bot: heuristic.NewBot(seed ^ 1)}, belief: b}
	seats := []seat.Seat{spy, heuristic.NewBot(seed ^ 2)}
	m, err := r.newMatch(tbl, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.slots = seats
	m.feeds = newMatchFeeds(seats)
	m.mu.Unlock()
	brd := botpolicy.NewBoard(2)
	roots, differ, fromBelief := 0, 0, 0
	for steps := 0; steps < 400; steps++ {
		n := len(spy.envs)
		_, pd := driveEnvStep(t, m, seats, &brd, nil)
		if pd == nil {
			break
		}
		if len(spy.envs) > n {
			rec := spy.envs[len(spy.envs)-1]
			setup := spy.setups[len(spy.setups)-1]
			if setup.Belief != b || setup.Decks[1] != nil || setup.Decks[0] == nil {
				t.Fatalf("seq %d: the closed seat's Setup is not closed: belief %v, decks %d/%d", rec.seq, setup.Belief != nil, len(setup.Decks[0]), len(setup.Decks[1]))
			}
			if rec.rootRef == "" {
				roots++
				m.mu.RLock()
				live := append(zoneNames(m.e.G.Zone(state.ZHand, 1), m.e), zoneNames(m.e.G.Zone(state.ZLibrary, 1), m.e)...)
				m.mu.RUnlock()
				root := append(append([]string(nil), rec.oppHand...), rec.oppLib...)
				if !sortedEqual(live, root) {
					differ++
				}
				for _, name := range root {
					if inBelief[name] {
						fromBelief++
						break
					}
				}
			}
		}
		submitEnvStep(t, r, tbl, m, pd)
	}
	t.Logf("%d closed roots; %d differ from the live hidden cards, %d dealt belief cards", roots, differ, fromBelief)
	if roots == 0 {
		t.Fatal("no closed root was built")
	}
	if differ != roots || fromBelief != roots {
		t.Fatalf("a closed root kept the live opponent's hidden cards (%d/%d differ, %d/%d from the belief)", differ, roots, fromBelief, roots)
	}
}
