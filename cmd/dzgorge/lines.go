package main

// PL-3 line probe (opt-in, play -lines): per game, per seat, the sequence of
// non-forced choices made in the first lineTurns turns (the "line") and the
// instant-timing counters of the 2026-10-14 INSTANT_LOG probe, kept in the
// game record. Read-only: it observes decisions and never changes an answer.

import (
	"slices"
	"strconv"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/rules"
	"github.com/adams-shaun/gorge/state"
)

// linesOn is play's -lines flag.
var linesOn bool

const lineTurns = 5

// timingRec counts one seat's instant timing: casts by window, and how often a
// castable instant met an opposing spell on the stack and was cast.
type timingRec struct {
	InstCasts       int `json:"inst_casts"`
	InstCastsOpp    int `json:"inst_casts_opp_turn"` // instant/flash casts in the opponent's turn
	OfferedOppSpell int `json:"offered_opp_spell"`   // priority windows, opp spell on top, an instant castable
	CastOppSpell    int `json:"cast_opp_spell"`      // ... of which the seat cast an instant
}

// lineRec is gameRec's -lines field.
type lineRec struct {
	Seq    [2][]string  `json:"seq"`
	Timing [2]timingRec `json:"timing"`
}

func instantish(o *state.Object) bool {
	if o == nil || o.Card == nil {
		return false
	}
	f := o.Face()
	return f != nil && (slices.Contains(f.Types, "Instant") || slices.Contains(f.Keywords, "Flash"))
}

func (l *lineRec) observe(e *rules.Engine, seatIdx int, d *decision.Decision, in decision.Intent) {
	if e == nil || len(d.Options) < 2 && in.Payment == nil {
		return
	}
	g := e.G
	if g.Turn <= lineTurns {
		l.Seq[seatIdx] = append(l.Seq[seatIdx], lineLabel(g, d, in, e))
	}
	if d.Kind != decision.KPriority {
		return
	}
	t := &l.Timing[seatIdx]
	mine := g.Active == d.Player
	oppSpell := false
	if n := len(g.Stack); n > 0 {
		if o := g.Obj(g.Stack[n-1]); o != nil {
			oppSpell = o.Controller != d.Player
		}
	}
	offered := false
	for _, o := range d.Options {
		if o.Kind == "cast" && instantish(g.Obj(o.Obj)) {
			offered = true
		}
	}
	var chosen state.ObjID
	for _, a := range e.EnsurePaymentActions() {
		if len(a.Plans) > 0 && instantish(g.Obj(a.Cast.Object)) {
			offered = true
		}
		if in.Payment != nil && a.ID == in.Payment.ActionID {
			chosen = a.Cast.Object
		}
	}
	if in.Payment == nil && len(in.Choices) == 1 && in.Choices[0] >= 0 && in.Choices[0] < len(d.Options) {
		if o := d.Options[in.Choices[0]]; o.Kind == "cast" {
			chosen = o.Obj
		}
	}
	cast := chosen != 0 && instantish(g.Obj(chosen))
	if oppSpell && (offered || cast) {
		t.OfferedOppSpell++
		if cast {
			t.CastOppSpell++
		}
	}
	if cast {
		t.InstCasts++
		if !mine {
			t.InstCastsOpp++
		}
	}
}

func lineLabel(g *state.Game, d *decision.Decision, in decision.Intent, e *rules.Engine) string {
	if in.Payment != nil {
		for _, a := range e.EnsurePaymentActions() {
			if a.ID == in.Payment.ActionID {
				if o := g.Obj(a.Cast.Object); o != nil && o.Face() != nil {
					return "t" + strconv.Itoa(int(g.Turn)) + ":pay:" + o.Face().Name
				}
			}
		}
		return "t" + strconv.Itoa(int(g.Turn)) + ":pay"
	}
	s := "t" + strconv.Itoa(int(g.Turn)) + ":" + string(d.Kind)
	for _, c := range in.Choices {
		if c >= 0 && c < len(d.Options) {
			s += "|" + d.Options[c].Kind + ":" + d.Options[c].Label
		}
	}
	return s
}
