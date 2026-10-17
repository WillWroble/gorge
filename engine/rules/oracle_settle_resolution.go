package rules

import (
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// passResolves returns the stack's top object when st is a `pass` step of a
// generated compliance scenario taken with a non-empty stack -- the pass that
// may complete a priority round and resolve that object (CR 117.4, 608.1) --
// and 0 otherwise.
func (r *oracleRun) passResolves(st oracleStep) state.ObjID {
	if !r.xmageFixture || oracleOpCodes.Code(string(st.Op)) != oracleOpPass {
		return 0
	}
	if n := len(r.e.G.Stack); n > 0 {
		return r.e.G.Stack[n-1]
	}
	return 0
}

// settleResolutionAsk finishes the resolution a `pass` step started when that
// resolution stopped on an ask of its own (Occult Epiphany's "then discard X
// cards", CR 608.2c), BEFORE the step's checkpoint. A `pass` answers exactly
// one priority decision, so gorge's checkpoint used to show the object still
// resolving -- drawn cards in hand, nothing discarded -- while XMage's
// pass pair waits until the object has resolved (the driver's
// waitStackResolved) and its checkpoint shows the finished resolution:
// Entropic Battlecruiser's static#0.0 diverged on p1.hand at that pass and
// agreed at every later checkpoint.
//
// Like settleTriggerAsks, the answers are the ones the scenario scripts on
// the NEXT step that has any, with what is left of them owed to that step
// (r.borrowed), and with none scripted each ask takes the harness's fallback,
// exactly as that later step would have. So only WHEN the ask is answered
// moves, never its outcome. It stops as soon as top has left the stack: the
// asks that follow a finished resolution (a trigger-order ask, a payment
// window) are settleTriggerAsks', and a turn-structure decision (declare
// attackers after a pass on an empty stack) never reaches here because top
// is 0 then.
func (r *oracleRun) settleResolutionAsk(steps []oracleStep, i int, top state.ObjID) error {
	if top == 0 || !r.midResolutionAsk(top) {
		return nil
	}
	next := -1
	for j := i + 1; j < len(steps); j++ {
		if len(steps[j].Answers) > 0 {
			next = j
			break
		}
	}
	saved := r.answers
	r.answers = nil
	if next >= 0 {
		if rest, ok := r.borrowed[next]; ok {
			r.answers = rest
		} else {
			r.answers = append([]oracleAnswer(nil), steps[next].Answers...)
		}
	}
	defer func() {
		if next >= 0 {
			if r.borrowed == nil {
				r.borrowed = map[int][]oracleAnswer{}
			}
			r.borrowed[next] = r.answers
		}
		r.answers = saved
	}()
	for n := 0; n < 100; n++ {
		if !r.midResolutionAsk(top) {
			return nil
		}
		if err := r.answer(r.e.Pending(), "settle resolution"); err != nil {
			return err
		}
	}
	return harnessf("settle: resolution ask never resolved")
}

// midResolutionAsk reports whether a non-priority decision is pending while
// top is still on the stack: the object a pass started resolving is waiting on
// an ask its own resolution posed.
func (r *oracleRun) midResolutionAsk(top state.ObjID) bool {
	d := r.e.Pending()
	if d == nil || d.Kind == decision.KPriority || r.e.G.Over {
		return false
	}
	for _, id := range r.e.G.Stack {
		if id == top {
			return true
		}
	}
	return false
}
