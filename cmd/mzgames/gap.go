package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/bench"
)

// The autotap gap (principle 3): at an autotap seat's priority decision,
// every cast the engine's potential-mana walk finds castable (the
// PotentialCasts census) should be offered, as XMage's autotap offers
// whatever all available mana can pay. A cast with neither a payment plan
// nor a legacy cast option is offered by nothing.

type gapStats struct {
	windows, gapWindows, gaps int
	byCard                    map[string]int
	// With OfferPotentialCasts on: the extra cast options offered, and what
	// playing each on a clone led to.
	extra, cast, reversed, failed int
}

// measureGap plays autotap games and counts unoffered castable casts.
func measureGap(out io.Writer, reg *cards.Registry, names []string, ds [][]*cards.Card, games int, seed uint64, offer bool) error {
	st := gapStats{byCard: map[string]int{}}
	for g := 0; g < games; g++ {
		s := seed + uint64(g)
		cfg := rules.Config{Seed: s, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
		seats, err := newSeats(s, true)
		if err != nil {
			return err
		}
		var e *rules.Engine
		hooks := bench.Hooks{
			Setup: func(x *rules.Engine) {
				e = x
				for p := range names {
					e.OfferPotentialCasts(state.PlayerID(p), offer)
				}
			},
			Decision: func(_ int, d *decision.Decision, _ decision.Intent, _ *botpolicy.Board) error {
				if d.Kind == decision.KPriority {
					st.add(e, d)
					if offer {
						st.tryExtras(e, d)
					}
				}
				return nil
			},
		}
		o, _, err := bench.PlayGame(cfg, seats, maxTurns, maxIntents, hooks)
		if err != nil {
			return fmt.Errorf("seed %d: %w", s, err)
		}
		if bench.IsAbort(o.StallOn) {
			return fmt.Errorf("seed %d: game aborted: %s", s, o.StallOn)
		}
	}
	fmt.Fprintf(out, "gap offer=%v %s-vs-%s games=%d priority_windows=%d windows_with_gap=%d unoffered_casts=%d extra_offered=%d extra_cast=%d extra_reversed=%d extra_failed=%d\n",
		offer, names[0], names[1], games, st.windows, st.gapWindows, st.gaps, st.extra, st.cast, st.reversed, st.failed)
	cs := make([]string, 0, len(st.byCard))
	for c := range st.byCard {
		cs = append(cs, c)
	}
	sort.Slice(cs, func(i, j int) bool { return st.byCard[cs[i]] > st.byCard[cs[j]] })
	for _, c := range cs {
		fmt.Fprintf(out, "  %5d  %s\n", st.byCard[c], c)
	}
	return nil
}

func (st *gapStats) add(e *rules.Engine, d *decision.Decision) {
	acts := e.EnsurePaymentActions()
	if !d.PotentialCastsKnown {
		return
	}
	st.windows++
	offered := map[state.ObjID]bool{}
	for _, o := range d.Options {
		if o.Kind == "cast" {
			offered[o.Obj] = true
		}
	}
	for _, a := range acts {
		if len(a.Plans) > 0 {
			offered[a.Cast.Object] = true
		}
	}
	n := 0
	for _, id := range d.PotentialCasts {
		if offered[id] || containsID(d.PotentialUnpayableCasts, id) {
			continue
		}
		n++
		name := "?"
		if o := e.G.Obj(id); o != nil && o.Face() != nil {
			name = o.Face().Name
		}
		st.byCard[name]++
	}
	if n > 0 {
		st.gapWindows++
		st.gaps += n
	}
}

func containsID(ids []state.ObjID, id state.ObjID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// tryExtras plays, on clones, each cast option the pool alone could not pay
// (OfferPotentialCasts' additions), answering what follows with gorge's
// autotap bot, and classifies the result: the card left its zone (cast),
// it is back where it started (reversed), or the engine refused.
func (st *gapStats) tryExtras(e *rules.Engine, d *decision.Decision) {
	for _, o := range d.Options[firstExtra(d):] {
		st.extra++
		c := e.CloneHypothetical(suffChanceSeed)
		from := c.G.Obj(o.Obj).Zone
		if c.SubmitHypothetical(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{o.Index}}) != nil {
			st.failed++
			continue
		}
		seats, _ := newSeats(7, true)
		ok := answerUntilPriority(c, d.Player, seats)
		switch {
		case !ok && !c.G.Over:
			st.failed++
		case c.G.Obj(o.Obj).Zone == from:
			st.reversed++
		default:
			st.cast++
		}
	}
}

// firstExtra is the index of OfferPotentialCasts' first addition: the
// engine's own priority list ends with concede, and the additions follow it.
func firstExtra(d *decision.Decision) int {
	for i, o := range d.Options {
		if o.Kind == "concede" {
			return i + 1
		}
	}
	return len(d.Options)
}
