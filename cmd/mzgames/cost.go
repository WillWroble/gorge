package main

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"time"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/testkit/bench"
)

// costTurns are the turns whose first real choice is measured: mid-game
// boards, as in cmd/enginebench's clone and step rows.
var costTurns = map[int32]bool{5: true, 6: true, 7: true, 8: true}

// costRow is the per-root cost of what one search node does on gorge: copy
// the engine, copy it and submit one intent, and project the omniscient view.
type costRow struct {
	clone, step, view []float64 // ns per op, one entry per root
}

// measureCost plays the games of one tap mode and times copies operations at
// the first choice point of each turn in costTurns. Timing is reported, never
// fed back into play, so the games are the same ones run prints.
func measureCost(out io.Writer, reg *cards.Registry, names []string, ds [][]*cards.Card, games int, seed uint64, auto bool, copies int) error {
	var r costRow
	for g := 0; g < games; g++ {
		s := seed + uint64(g)
		cfg := rules.Config{Seed: s, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
		seats := make([]seat.Seat, 2)
		for i := range seats {
			st, err := bots.New(bots.Default, bots.Options{Seed: s ^ uint64(i+1), AutoPayMana: auto})
			if err != nil {
				return err
			}
			seats[i] = st
		}
		var e *rules.Engine
		seen := map[int32]bool{}
		hooks := bench.Hooks{
			Setup: func(x *rules.Engine) { e = x },
			Decision: func(_ int, d *decision.Decision, in decision.Intent, _ *botpolicy.Board) error {
				if !costTurns[e.G.Turn] || seen[e.G.Turn] || !choicePoint(d) {
					return nil
				}
				seen[e.G.Turn] = true
				r.add(e, d, in, copies)
				return nil
			},
		}
		if _, _, err := bench.PlayGame(cfg, seats, maxTurns, maxIntents, hooks); err != nil {
			return fmt.Errorf("seed %d: %w", s, err)
		}
	}
	mode := "manual"
	if auto {
		mode = "auto"
	}
	fmt.Fprintf(out, "cost tap=%s %s-vs-%s roots=%d copies=%d clone_ns=%s step_ns=%s view_ns=%s\n",
		mode, names[0], names[1], len(r.clone), copies, stat(r.clone), stat(r.step), stat(r.view))
	return nil
}

func (r *costRow) add(e *rules.Engine, d *decision.Decision, in decision.Intent, copies int) {
	var sp rules.Spare
	runtime.GC()
	t := time.Now()
	for i := 0; i < copies; i++ {
		sp = e.CloneInto(&sp).Release()
	}
	r.clone = append(r.clone, perOp(t, copies))

	t = time.Now()
	for i := 0; i < copies; i++ {
		c := e.CloneInto(&sp)
		_ = c.Submit(in)
		sp = c.Release()
	}
	r.step = append(r.step, perOp(t, copies))

	t = time.Now()
	for i := 0; i < copies; i++ {
		_ = view.ProjectFor(e.G, e, view.NoSeat, view.Omniscient, d)
	}
	r.view = append(r.view, perOp(t, copies))
}

func perOp(t time.Time, n int) float64 { return float64(time.Since(t).Nanoseconds()) / float64(n) }

// stat formats median/mean over roots.
func stat(xs []float64) string {
	if len(xs) == 0 {
		return "-"
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	sum := 0.0
	for _, x := range s {
		sum += x
	}
	return fmt.Sprintf("%.0f/%.0f", s[len(s)/2], sum/float64(len(s)))
}

// choicePoint is a decision with something to choose beyond passing or a
// bare mana tap (cmd/enginebench's definition).
func choicePoint(d *decision.Decision) bool {
	if d.Kind != decision.KPriority {
		return true
	}
	for _, o := range d.Options {
		switch o.Kind {
		case "pass", "concede", "activate":
		default:
			return true
		}
	}
	return false
}
