package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/testkit/bench"
)

// The view-sufficiency probe tests principle 1 (everything dynamic is in the
// view) empirically. At priority decisions of real games it plays two of the
// offered actions in both orders on clones. When both orders reach the same
// omniscient view, both clones get the same future chance and the same fresh
// seats, and are played forward side by side. Seats answer from the view, so
// if the views ever differ afterwards, state that changes play was missing
// from the view at the fork.

const (
	suffPairsPerDecision = 3
	suffSteps            = 80
	suffChanceSeed       = 1
)

type suffStats struct {
	pairs, transpositions, divergent int
	diverged                         map[string]int // first differing view path -> count
	example                          map[string]string
}

// probeSufficiency plays the games of one tap mode and probes every priority
// decision. It prints a summary line and the divergent paths.
func probeSufficiency(out io.Writer, reg *cards.Registry, names []string, ds [][]*cards.Card, games int, seed uint64, auto bool) error {
	st := suffStats{diverged: map[string]int{}, example: map[string]string{}}
	for g := 0; g < games; g++ {
		s := seed + uint64(g)
		cfg := rules.Config{Seed: s, Names: names, Decks: ds, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
		seats, err := newSeats(s, auto)
		if err != nil {
			return err
		}
		var e *rules.Engine
		hooks := bench.Hooks{
			Setup: func(x *rules.Engine) { e = x },
			Decision: func(_ int, d *decision.Decision, _ decision.Intent, _ *botpolicy.Board) error {
				if d.Kind == decision.KPriority {
					st.probe(e, d, auto)
				}
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
	fmt.Fprintf(out, "suff tap=%s %s-vs-%s games=%d pairs=%d transpositions=%d divergent=%d\n",
		mode, names[0], names[1], games, st.pairs, st.transpositions, st.divergent)
	paths := make([]string, 0, len(st.diverged))
	for p := range st.diverged {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool { return st.diverged[paths[i]] > st.diverged[paths[j]] })
	for _, p := range paths {
		fmt.Fprintf(out, "  %4d  %s\n        e.g. %s\n", st.diverged[p], p, st.example[p])
	}
	return nil
}

func newSeats(seed uint64, auto bool) ([]seat.Seat, error) {
	seats := make([]seat.Seat, 2)
	for i := range seats {
		s, err := bots.New(bots.Default, bots.Options{Seed: seed ^ uint64(i+1), AutoPayMana: auto})
		if err != nil {
			return nil, err
		}
		seats[i] = s
	}
	return seats, nil
}

func (st *suffStats) probe(e *rules.Engine, d *decision.Decision, auto bool) {
	var acts []int
	for _, o := range d.Options {
		if o.Kind != "pass" && o.Kind != "concede" {
			acts = append(acts, o.Index)
		}
	}
	n := 0
	for i := 0; i < len(acts) && n < suffPairsPerDecision; i++ {
		for j := i + 1; j < len(acts) && n < suffPairsPerDecision; j++ {
			n++
			st.pairs++
			a, b := d.Options[acts[i]], d.Options[acts[j]]
			c1, ok1 := playBoth(e, a, b, auto)
			c2, ok2 := playBoth(e, b, a, auto)
			if !ok1 || !ok2 || viewKey(c1) != viewKey(c2) {
				continue
			}
			st.transpositions++
			if path, ex := st.drive(c1, c2, auto); path != "" {
				st.divergent++
				st.diverged[path]++
				if _, ok := st.example[path]; !ok {
					st.example[path] = fmt.Sprintf("turn %d, %q then %q: %s", e.G.Turn, a.Label, b.Label, ex)
				}
			}
		}
	}
}

// playBoth plays x, then the option matching y, on a fresh hypothetical
// clone. Decisions in between (targets, modes, payment, the opponent) are
// answered by fresh deterministic seats until the same player has priority
// again.
func playBoth(e *rules.Engine, x, y decision.Option, auto bool) (*rules.Engine, bool) {
	p := e.Pending().Player
	c := e.CloneHypothetical(suffChanceSeed)
	seats, _ := newSeats(7, auto)
	for _, o := range []decision.Option{x, y} {
		if !submitMatching(c, o) || !answerUntilPriority(c, p, seats) {
			return nil, false
		}
	}
	return c, true
}

// submitMatching submits the pending priority option that matches o.
func submitMatching(c *rules.Engine, o decision.Option) bool {
	d := c.Pending()
	if d == nil || d.Kind != decision.KPriority {
		return false
	}
	for _, x := range d.Options {
		if optionKey(x) == optionKey(o) {
			return submitOption(c, x)
		}
	}
	return false
}

// answerUntilPriority answers decisions until p holds priority again.
func answerUntilPriority(c *rules.Engine, p state.PlayerID, seats []seat.Seat) bool {
	for k := 0; k < 20; k++ {
		d := c.Pending()
		if c.G.Over || d == nil {
			return false
		}
		if d.Kind == decision.KPriority && d.Player == p {
			return true
		}
		in, err := decide(c, seats)
		if err != nil || c.SubmitHypothetical(in) != nil {
			return false
		}
	}
	return false
}

func submitOption(c *rules.Engine, o decision.Option) bool {
	d := c.Pending()
	return c.SubmitHypothetical(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{o.Index}}) == nil
}

// optionKey identifies the same offered action across two decisions.
func optionKey(o decision.Option) string {
	return fmt.Sprint(o.Kind, o.Obj, o.Player, o.Ability, o.AltCostIndex, o.Mode, o.SVar, o.Keyword, o.ManaSymbol, o.Cost, o.Label)
}

// drive plays both clones forward with identical fresh seats and returns the
// first differing view path, or "" when they stay identical.
func (st *suffStats) drive(c1, c2 *rules.Engine, auto bool) (path, example string) {
	s1, _ := newSeats(7, auto)
	s2, _ := newSeats(7, auto)
	for k := 0; k < suffSteps; k++ {
		k1, k2 := viewKey(c1), viewKey(c2)
		if k1 != k2 {
			return firstDiff(k1, k2)
		}
		if c1.G.Over || c1.Pending() == nil {
			return "", ""
		}
		in1, err1 := decide(c1, s1)
		in2, err2 := decide(c2, s2)
		if err1 != nil || err2 != nil {
			return "", ""
		}
		e1, e2 := c1.SubmitHypothetical(in1), c2.SubmitHypothetical(in2)
		if (e1 == nil) != (e2 == nil) {
			return "submit-result", fmt.Sprintf("%v vs %v", e1, e2)
		}
		if e1 != nil {
			return "", ""
		}
	}
	return "", ""
}

func decide(c *rules.Engine, seats []seat.Seat) (decision.Intent, error) {
	d := c.Pending()
	s := seats[d.Player]
	if pc, ok := s.(seat.PaymentPlanConsumer); ok && pc.WantsPaymentActions() {
		c.EnsurePaymentActions()
	}
	v := view.Project(c.G, c, d.Player, d)
	return s.Decide(context.Background(), v, *d)
}

// viewKey is the omniscient view as canonical JSON, without the fields that
// count decisions or cache derived payment data.
func viewKey(c *rules.Engine) string {
	var dc *decision.Decision
	if d := c.Pending(); d != nil {
		x := d.CloneValue()
		x.Seq = 0
		x.PaymentActions, x.PaymentActionsBuilt = nil, false
		x.PotentialCasts, x.PotentialUnpayableCasts, x.PotentialCastsKnown = nil, nil, false
		dc = &x
	}
	v := view.ProjectDynamic(c.G, c, view.NoSeat, view.Omniscient, nil, dc)
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// firstDiff names the first JSON path where two view keys differ, with list
// indices folded to [] so one gap counts once.
func firstDiff(a, b string) (string, string) {
	var x, y any
	_ = json.Unmarshal([]byte(a), &x)
	_ = json.Unmarshal([]byte(b), &y)
	return diffAt("", x, y)
}

func diffAt(path string, x, y any) (string, string) {
	if reflect.DeepEqual(x, y) {
		return "", ""
	}
	switch xv := x.(type) {
	case map[string]any:
		if yv, ok := y.(map[string]any); ok {
			keys := make([]string, 0, len(xv)+len(yv))
			for k := range xv {
				keys = append(keys, k)
			}
			for k := range yv {
				if _, ok := xv[k]; !ok {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				if p, ex := diffAt(path+"."+k, xv[k], yv[k]); p != "" {
					return p, ex
				}
			}
		}
	case []any:
		if yv, ok := y.([]any); ok && len(xv) == len(yv) {
			for i := range xv {
				if p, ex := diffAt(path+"[]", xv[i], yv[i]); p != "" {
					return p, ex
				}
			}
		}
	}
	return path, truncate(fmt.Sprintf("%v vs %v", x, y), 200)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
