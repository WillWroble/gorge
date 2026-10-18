package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
)

// TestHypCloneOfACloneNeverKeepsAStalePrefix: a clone of a clone, released
// into the pool and reused under the same parent array's next tenant, must
// copy that tenant's history, not keep the events its old parent had
// appended (events.Log.CloneIntoFrom's provenance skip). The witness search
// for census pair 1905 hit exactly this at depth three.
func TestHypCloneOfACloneNeverKeepsAStalePrefix(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Grizzly Bears")
	r1 := e.hypClone(e)
	c1 := e.hypClone(r1)
	c1.emit(events.Event{Kind: events.Note, Text: "c1-a"})
	c1.emit(events.Event{Kind: events.Note, Text: "c1-b"})
	g1 := e.hypClone(c1)
	e.hypRelease(g1)
	e.hypRelease(c1)
	e.hypRelease(r1)

	e.emit(events.Event{Kind: events.Note, Text: "e-1"})
	e.emit(events.Event{Kind: events.Note, Text: "e-2"})
	e.emit(events.Event{Kind: events.Note, Text: "e-3"})
	r2 := e.hypClone(e)
	c2 := e.hypClone(r2)
	g2 := e.hypClone(c2)
	if len(g2.L.Events) != len(c2.L.Events) || g2.L.Head() != c2.L.Head() {
		t.Fatalf("grandchild log %d events head %s, parent %d head %s", len(g2.L.Events), g2.L.Head(), len(c2.L.Events), c2.L.Head())
	}
	for i := range c2.L.Events {
		if g2.L.Events[i].Kind != c2.L.Events[i].Kind || g2.L.Events[i].Text != c2.L.Events[i].Text {
			t.Fatalf("grandchild event %d = %s %q, parent has %s %q", i, g2.L.Events[i].Kind, g2.L.Events[i].Text, c2.L.Events[i].Kind, c2.L.Events[i].Text)
		}
	}
	e.hypRelease(g2)
	e.hypRelease(c2)
	e.hypRelease(r2)
}
