// Package atomic factorizes a gorge decision into the atomic decisions of
// the mage fork's MageZero (ActionEncoder.ActionType): every decision is
// answered one pick at a time, each pick its own search node (principle 2
// of mz-gorge-design.md). Combat is one creature at a time in object order,
// as in XMage's selectAttackersOneAtATime and selectBlockersOneAtATime; a
// multi-pick is repeated CHOOSE_TARGET picks with stop allowed once the
// minimum is met; modes and X are CHOOSE_NUM, yes/no CHOOSE_USE, a named
// choice MAKE_CHOICE.
//
// The half-made answer is the engine's (Decision.Staged and Declined, set
// through Stage and Decline), so the next atom is a function of the
// decision alone and nothing is kept here.
package atomic

import (
	"slices"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
)

// Type is an atomic decision type, XMage's ActionEncoder.ActionType.
type Type uint8

const (
	Priority Type = iota
	ChooseTarget
	ChooseUse
	ChooseNum
	MakeChoice
)

func (t Type) String() string {
	return [...]string{"PRIORITY", "CHOOSE_TARGET", "CHOOSE_USE", "CHOOSE_NUM", "MAKE_CHOICE"}[t]
}

// Stop is the choice that picks no option: stop choosing, or for a combat
// atom, the creature takes no part.
const Stop = -1

// An Atom is the next atomic decision of a pending decision.
type Atom struct {
	Type Type
	// Subject is the attacker or blocker a combat atom decides, else 0.
	Subject state.ObjID
	// Options are the decision's option indices the atom chooses among.
	Options []int
	// CanStop reports whether Stop is also a choice.
	CanStop bool
}

// Choices is the number of choices the atom offers.
func (a Atom) Choices() int {
	if a.CanStop {
		return len(a.Options) + 1
	}
	return len(a.Options)
}

// Next returns d's next atom. done reports that d's staged answer is
// complete: it only remains to commit it.
func Next(d *decision.Decision) (a Atom, done bool) {
	switch d.Kind {
	case decision.KAttackers, decision.KBlockers:
		return nextCombat(d)
	}
	if len(d.Staged) >= d.Max && d.Max > 0 {
		return Atom{}, true
	}
	a.Type = typeOf(d)
	for i := range d.Options {
		if d.Repeatable || !slices.Contains(d.Staged, i) {
			a.Options = append(a.Options, i)
		}
	}
	a.CanStop = len(d.Staged) >= d.Min || (d.AllowNone && len(d.Staged) == 0)
	if len(a.Options) == 0 {
		return Atom{}, true
	}
	return a, false
}

// nextCombat is the next undecided attacker or blocker, lowest object first.
func nextCombat(d *decision.Decision) (Atom, bool) {
	decided := func(obj state.ObjID) bool {
		if slices.Contains(d.Declined, obj) {
			return true
		}
		for _, j := range d.Staged {
			if d.Options[j].Obj == obj {
				return true
			}
		}
		return false
	}
	var subject state.ObjID
	for _, o := range d.Options {
		if !decided(o.Obj) && (subject == 0 || o.Obj < subject) {
			subject = o.Obj
		}
	}
	if subject == 0 {
		return Atom{}, true
	}
	a := Atom{Type: ChooseTarget, Subject: subject, CanStop: true}
	for i, o := range d.Options {
		if o.Obj == subject {
			a.Options = append(a.Options, i)
		}
	}
	return a, false
}

// typeOf maps a decision kind, and for a choose its option vocabulary, onto
// the atomic type of its picks.
func typeOf(d *decision.Decision) Type {
	switch d.Kind {
	case decision.KPriority:
		return Priority
	case decision.KModes:
		return ChooseNum
	case decision.KTriggerOptional, decision.KCommanderZone:
		return ChooseUse
	case decision.KMulligan:
		if d.Max == 1 {
			return ChooseUse
		}
	case decision.KReplacement:
		if len(d.Options) > 0 && d.Options[0].Kind == "apply" {
			return ChooseUse
		}
	case decision.KStartingPlayer:
		return MakeChoice
	case decision.KChoose:
		if len(d.Options) > 0 {
			switch d.Options[0].Kind {
			case "x", "number":
				return ChooseNum
			case "yes", "no":
				return ChooseUse
			case "name", "type", "dungeon", "room", "mana", "color":
				return MakeChoice
			}
		}
	}
	return ChooseTarget
}

// Engine is what Apply plays a choice on: rules.Engine.
type Engine interface {
	Pending() *decision.Decision
	Stage(int) error
	Decline(state.ObjID) error
	Commit() error
}

// Apply plays choice c (an option index of a, or Stop) on e's pending
// decision, and commits the answer once it is complete.
func Apply(e Engine, a Atom, c int) error {
	var err error
	switch {
	case c == Stop && a.Subject != 0:
		err = e.Decline(a.Subject)
	case c == Stop:
		return e.Commit()
	default:
		err = e.Stage(c)
	}
	if err != nil {
		return err
	}
	if _, done := Next(e.Pending()); done {
		return e.Commit()
	}
	return nil
}
