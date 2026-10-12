package effects

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// putCounterExisting runs the CounterType$ ExistingCounter shape (task
// lb-existctr): ExistingCounter is not a counter kind but "a kind of counter
// the recipient already has" (CR 122.1, 122.6). With EachExistingCounter$
// True (Powerful Broker, Maulfist Revolutionary, Skyship Plunderer: "for each
// kind of counter on target permanent or player, give that permanent or
// player another counter of that kind") every kind the recipient carries
// takes CounterNum$ more; without it (Animation Module, Ichormoon Gauntlet,
// The Caves of Androzani: "choose a counter on ...") the controller picks ONE
// of those kinds through the shared counter_kind ask. A recipient with no
// counters takes nothing. The kinds are read in the recipient's counter-slice
// order before any placement (never a map walk), so the walk is
// deterministic and a kind added mid-walk is never revisited.
func putCounterExisting(h Host, c *Ctx, sa *cards.SA, pc *PutCounterParams, n int32) {
	g := h.Game()
	var placed []state.Target
	for ti, t := range Defined(h, c, sa) {
		var cs []state.Counter
		var p state.PlayerID
		if t.IsPlayer {
			p = PlayerOf(h, c, t)
			if int(p) < 0 || int(p) >= len(g.Players) {
				continue
			}
			cs = g.Players[p].Counters
		} else {
			o := g.Obj(t.Obj)
			if o == nil {
				continue
			}
			cs = o.Counters
		}
		kinds := counterKinds("", len(cs),
			func(i int) string { return cs[i].Kind },
			func(i int) int32 { return cs[i].N })
		if len(kinds) == 0 {
			continue
		}
		if !pc.EachExistingCounter {
			kinds = []string{putCounterExistingPick(h, c, sa, ti, kinds)}
		}
		for _, k := range kinds {
			if t.IsPlayer {
				emitPutCounterChange(h, c, sa, events.Event{Kind: events.PlayerCounterChange, Player: p, Counter: k, Amount: n})
			} else {
				emitPutCounterChange(h, c, sa, events.Event{Kind: events.CounterChange, Obj: t.Obj, Counter: k, Amount: n})
			}
		}
		// RememberPut$'s contract (effPutCounter's loop): only a recipient
		// that actually took a counter; a player only under RememberPut$.
		if n > 0 && (!t.IsPlayer || pc.RememberPut) {
			if t.IsPlayer {
				t = state.Target{Player: p, IsPlayer: true}
			}
			placed = append(placed, t)
		}
	}
	rememberPlaced(c, sa, placed)
}

// putCounterExistingPick is the "choose a counter on it" ask over the kinds
// recipient ti carries: no ask when there is one kind; otherwise the shared
// counter_kind KChoose (Min 1, Max 1, botpolicy's first-option arm), whose
// unserved stand-in is the same first kind.
func putCounterExistingPick(h Host, c *Ctx, sa *cards.SA, ti int, kinds []string) string {
	if len(kinds) == 1 {
		return kinds[0]
	}
	d := &decision.Decision{Player: c.Controller, Kind: decision.KChoose, Min: 1, Max: 1, Source: c.Source,
		ResumeKind: "counter_kind", ResumeSA: sa, ResumeTarget: ti, Prompt: "Choose a kind of counter"}
	for i, k := range kinds {
		d.Options = append(d.Options, decision.Option{Index: i, Kind: "counter", Label: k, Player: c.Controller})
	}
	if ans, ok := AskTape(h, d); ok {
		if k := counterAnswerLabel(ans); k != "" {
			return k
		}
	}
	return kinds[0]
}
