package main

import (
	"sync"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
)

// handLog records, per game, the eligible cards each seat held in hand:
// its opening hand once the mulligans are done, plus every card it later
// moved from its library to its hand (draws and tutors). That is 17Lands'
// "game in hand" set (GIH), so a card's in-hand win rate can be read off the
// same games the deck-composition regression uses.
//
// OnEngine runs inside PlayGame on the playing goroutine, so the entry is
// keyed by the game's seed and play takes it back right after the game. The
// two games of a swap trial share a seed but are played one after the other
// on one goroutine.
type handLog struct {
	idx map[string]int // card name -> index into cards.json
	m   sync.Map       // seed -> [2][]int
}

func newHandLog(cs []card) *handLog {
	h := &handLog{idx: make(map[string]int, len(cs))}
	for i, c := range cs {
		h.idx[c.Name] = i
	}
	return h
}

func (h *handLog) add(seed uint64, eng *rules.Engine) {
	inHand := map[state.ObjID]bool{}
	seen := map[state.ObjID]bool{}
	started := false
	for _, e := range eng.L.Events {
		switch e.Kind {
		case events.TurnChange:
			if !started {
				started = true
				for id := range inHand {
					seen[id] = true
				}
			}
		case events.Draw, events.MoveZone:
			if e.To == state.ZHand {
				inHand[e.Obj] = true
				if started && e.From == state.ZLibrary {
					seen[e.Obj] = true
				}
			} else {
				delete(inHand, e.Obj)
			}
		}
	}
	var out [2][]int
	for id := range seen {
		o := eng.G.Obj(id)
		if o == nil || o.Card == nil || len(o.Card.Faces) == 0 || o.Owner < 0 || int(o.Owner) > 1 {
			continue
		}
		if i, ok := h.idx[o.Card.Faces[0].Name]; ok {
			out[o.Owner] = append(out[o.Owner], i)
		}
	}
	h.m.Store(seed, out)
}

// take returns and forgets the game's in-hand sets.
func (h *handLog) take(seed uint64) ([2][]int, bool) {
	v, ok := h.m.LoadAndDelete(seed)
	if !ok {
		return [2][]int{}, false
	}
	return v.([2][]int), true
}
