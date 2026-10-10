package main

import (
	"context"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/internal/corpuscov"
	"github.com/adams-shaun/gorge/internal/searchprobe"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/seat"
	"github.com/adams-shaun/gorge/state"
	"github.com/adams-shaun/gorge/view"
)

// exploreDriver is -explore: coverage-directed exploration of the search
// seat's priority decisions (corpuscov.ExploreSeat). With probability P a
// decision's played answer is the least-chosen offered (card, ability) slot
// instead of the teacher's/bot's answer, and the decision's label (when the
// teacher covered it) carries explore:true so policy imitation drops it.
// The census is per game, so a corpus built with any -workers value is
// unchanged; every random draw comes from a PCG seeded by the game seed.
type exploreDriver struct {
	seat *corpuscov.ExploreSeat
	inn  *answerSeat
	cen  *corpuscov.Census
}

// answerSeat is the wrapped seat: it plays whatever answer the teacher/bot
// already chose, so a non-forced decision is unchanged.
type answerSeat struct{ in decision.Intent }

func (a *answerSeat) Decide(context.Context, view.View, decision.Decision) (decision.Intent, error) {
	return a.in, nil
}

var _ seat.Seat = (*answerSeat)(nil)

// newExploreDriver returns nil when p <= 0 (exploration off; callers skip).
func newExploreDriver(p float64, setup searchprobe.PublicGame, seed uint64, searchSeat int, e *rules.Engine) *exploreDriver {
	if p <= 0 {
		return nil
	}
	x := &exploreDriver{inn: &answerSeat{}, cen: corpuscov.New()}
	x.cen.BeginGame(setup.Names, setup.Decks)
	x.seat = corpuscov.NewExploreSeat(x.inn, p, seed*0x9e3779b97f4a7c15^uint64(searchSeat+1), x.cen, func() *state.Game { return e.G })
	return x
}

// answer returns the intent to play at d (in unless forced) and whether it
// was forced exploration.
func (x *exploreDriver) answer(e *rules.Engine, d *decision.Decision, in decision.Intent) (decision.Intent, bool) {
	x.inn.in = in
	out, err := x.seat.Decide(context.Background(), view.View{}, *d)
	if err != nil {
		return in, false
	}
	forced := x.seat.Explored()
	x.cen.ObserveChoice(e.G, d, out, forced)
	return out, forced
}
