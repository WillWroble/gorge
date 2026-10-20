package main

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"time"

	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/view"
)

// The atomic test vehicle plays whole games with both seats choosing at
// random, one atomic pick at a time, through rules' Stage/Decline/Commit
// (principle 2). It counts the composed answers the engine rejects (the
// prunes principle 3 allows) and times each atomic step.

const atomicMaxDecisions = 20000

type atomicStats struct {
	games, decisions, atoms, rejected, aborted int
	byType                                     [5]int
	elapsed                                    time.Duration
}

func runAtomic(out io.Writer, reg *cards.Registry, names []string, games int, seed uint64, tap string) error {
	ds, err := loadPair(reg, names)
	if err != nil {
		return err
	}
	modes := map[string][]bool{"auto": {true}, "manual": {false}, "both": {true, false}}[tap]
	for _, auto := range modes {
		var st atomicStats
		for g := 0; g < games; g++ {
			if err := st.play(reg, names, ds, seed+uint64(g), auto); err != nil {
				return err
			}
		}
		mode := "manual"
		if auto {
			mode = "auto"
		}
		fmt.Fprintf(out, "atomic tap=%s %s-vs-%s games=%d aborted=%d decisions=%d atoms=%d rejected=%d ns_per_atom=%.0f",
			mode, names[0], names[1], st.games, st.aborted, st.decisions, st.atoms, st.rejected, float64(st.elapsed.Nanoseconds())/float64(max(st.atoms, 1)))
		for t, n := range st.byType {
			fmt.Fprintf(out, " %s=%d", atomic.Type(t), n)
		}
		fmt.Fprintln(out)
	}
	return nil
}

// play plays one game; a livelock or other engine panic aborts it.
func (st *atomicStats) play(reg *cards.Registry, names []string, ds [][]*cards.Card, seed uint64, auto bool) (err error) {
	cfg := rules.Config{Seed: seed, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
	cfg.LoopGuard = rules.WithDefaultObjectCap(cfg.LoopGuard)
	e := rules.New(cfg)
	e.Advance()
	rng := rand.New(rand.NewPCG(seed, 7))
	fallback, err := newSeats(seed, auto)
	if err != nil {
		return err
	}
	st.games++
	defer func() {
		if r := recover(); r != nil {
			st.aborted++
		}
	}()
	for n := 0; !e.G.Over && e.Pending() != nil && n < atomicMaxDecisions; n++ {
		st.decisions++
		if auto {
			e.EnsurePotentialCasts()
		}
		d := e.Pending()
		if delegate(d, auto) {
			if err := submitSeat(e, fallback); err != nil {
				return err
			}
			continue
		}
		seq := d.Seq
		for e.Pending() != nil && e.Pending().Seq == seq {
			a, done := atomic.Next(e.Pending())
			t := time.Now()
			var err error
			if done {
				err = e.Commit()
			} else {
				st.byType[a.Type]++
				err = atomic.Apply(e, a, pick(rng, e.Pending(), a, auto))
			}
			st.elapsed += time.Since(t)
			st.atoms++
			if err != nil {
				// An illegal composed answer: prune it, and answer this
				// decision as gorge's bot would.
				st.rejected++
				if err := submitSeat(e, fallback); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// pick chooses one of a's choices at random. A priority atom never concedes,
// and in autotap never taps for mana: those are not playables (XMage).
func pick(rng *rand.Rand, d *decision.Decision, a atomic.Atom, auto bool) int {
	var cs []int
	for _, i := range a.Options {
		switch k := d.Options[i].Kind; {
		case a.Type == atomic.Priority && k == "concede":
		case a.Type == atomic.Priority && auto && k == "activate":
		default:
			cs = append(cs, i)
		}
	}
	if a.CanStop {
		cs = append(cs, atomic.Stop)
	}
	return cs[rng.IntN(len(cs))]
}

// delegate reports whether gorge's bot answers d: in autotap, the CR 601.2g
// mana window, which XMage's auto-tapper answers without a node.
func delegate(d *decision.Decision, auto bool) bool {
	if !auto || d.Kind != decision.KChoose {
		return false
	}
	for _, o := range d.Options {
		if o.Kind == "mana" {
			return true
		}
	}
	return false
}

func submitSeat(e *rules.Engine, seats []seat.Seat) error {
	d := e.Pending()
	s := seats[d.Player]
	if pc, ok := s.(seat.PaymentPlanConsumer); ok && pc.WantsPaymentActions() {
		e.EnsurePaymentActions()
	}
	in, err := s.Decide(context.Background(), view.Project(e.G, e, d.Player, d), *d)
	if err != nil {
		return err
	}
	return e.Submit(in)
}
