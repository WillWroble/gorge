package rules

import (
	"github.com/adams-shaun/gorge/engine/state"
)

// fireTimeRememberedStamps stamps every remembered OBJECT with its
// Incarnation as the trigger fired -- the state the synchronous emit check
// sees, right after the triggering event applied. It rides the queued
// trigger's effects.TriggerContext.TriggerRememberedStamps so a body that
// resolves later (a dies trigger waiting on the stack through a round trip)
// stamps a delayed registration with the incarnation the triggering event
// left, not the one the object carries at resolution: the trigger refers to
// the object that existed when it fired (CR 400.7). Player references and
// objects that no longer resolve carry no stamp, the same shape
// events.rememberedStampsFrom keeps.
func (e *Engine) fireTimeRememberedStamps(remembered []state.Target) []state.TargetStamp {
	var out []state.TargetStamp
	for _, t := range remembered {
		if t.IsPlayer {
			continue
		}
		if o := e.G.Obj(t.Obj); o != nil {
			out = append(out, state.TargetStamp{Obj: t.Obj, Incarnation: o.Incarnation})
		}
	}
	return out
}
