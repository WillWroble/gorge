package trigmatch

import (
	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// specializesMatches is Mode$ Specializes: "Whenever this permanent
// specializes". The specializing permanent is the event's Obj (events.
// Specialize), so the matcher fires on the object itself, exactly as
// TurnFaceUp fires on the turned permanent.
func specializesMatches(e Board, t cards.Trigger, source state.ObjID, ev events.Event, _ *state.Object) bool {
	return ev.Kind == events.Specialize && ev.Obj == source
}

func init() {
	registerTrigMatcher(specializesMatches, "Specializes")
}
