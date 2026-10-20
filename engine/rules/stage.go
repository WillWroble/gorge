package rules

import (
	"errors"
	"fmt"
	"slices"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// Stage, Decline and Commit answer the pending decision one atomic pick at a
// time: the half-made answer is held on the decision (Decision.Staged and
// Declined), so it is game state, in the view and in a Clone, and two orders
// of the same picks reach the same state unless the decision's answer is
// ordered. mz-gorge's bot factorizes decisions this way in its search
// (principle 2 of mz-gorge-design.md, XMage's one-attacker-at-a-time
// combat); no other seat stages, so no other decision stream changes. A
// pick is checked only for what can never become legal; Commit's Submit
// checks the whole answer, and a rejected answer leaves the decision posed.

var errNoDecision = errors.New("rules: no pending decision")

// Stage picks option i of the pending decision.
func (e *Engine) Stage(i int) error {
	d := e.pending
	if d == nil {
		return errNoDecision
	}
	if i < 0 || i >= len(d.Options) {
		return fmt.Errorf("rules: stage option %d of %d", i, len(d.Options))
	}
	if len(d.Staged) >= d.Max {
		return fmt.Errorf("rules: stage beyond max %d", d.Max)
	}
	o := &d.Options[i]
	for _, j := range d.Staged {
		if j == i && !d.Repeatable {
			return fmt.Errorf("rules: option %d already staged", i)
		}
		if g := d.Options[j].Group; g != "" && g == o.Group && j != i {
			return fmt.Errorf("rules: option %d shares group %q with staged option %d", i, g, j)
		}
	}
	if o.Obj != 0 && slices.Contains(d.Declined, o.Obj) {
		return fmt.Errorf("rules: option %d names declined object %d", i, o.Obj)
	}
	staged := append(slices.Clone(d.Staged), i)
	if !d.OrderedAnswer() {
		slices.Sort(staged)
	}
	d.Staged = staged
	return nil
}

// Decline records that obj takes none of the pending decision's options
// (an attacker that does not attack, a blocker that does not block).
func (e *Engine) Decline(obj state.ObjID) error {
	d := e.pending
	if d == nil {
		return errNoDecision
	}
	if slices.Contains(d.Declined, obj) {
		return fmt.Errorf("rules: object %d already declined", obj)
	}
	for _, j := range d.Staged {
		if d.Options[j].Obj == obj {
			return fmt.Errorf("rules: object %d has a staged option", obj)
		}
	}
	declined := append(slices.Clone(d.Declined), obj)
	slices.Sort(declined)
	d.Declined = declined
	return nil
}

// Commit answers the pending decision with its staged picks, through
// SubmitHypothetical on a hypothetical engine.
func (e *Engine) Commit() error {
	d := e.pending
	if d == nil {
		return errNoDecision
	}
	in := decision.Intent{Seq: d.Seq, Player: d.Player, Choices: slices.Clone(d.Staged)}
	if e.rng != nil && e.rng.chance != nil {
		return e.SubmitHypothetical(in)
	}
	return e.Submit(in)
}
