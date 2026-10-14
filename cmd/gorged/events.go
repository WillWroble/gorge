package main

import (
	"fmt"
	"path/filepath"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/eventhost"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/host/httpapi"
	"github.com/adams-shaun/gorge/state"
)

// eventhost.Host already satisfies the /api/events backend interface; the
// assertion keeps the two packages' shapes in step when either changes.
var _ httpapi.EventService = (*eventhost.Host)(nil)

// eventsWiring is what serve builds before host.New (the store whose deck
// loader the registry needs, and the OnMatchEnd relay) for startEvents.
// The zero value means -events is off.
type eventsWiring struct {
	store *eventhost.Store
	relay *eventhost.Relay
}

// prepareEvents opens the event store under <dir>/events (memory when dir
// is "") and chains its deck loader in front of load, so a host deck load
// of an event deck id reads the event's saved deck file before the ordinary
// loaders see the name. With -events off it returns load unchanged and a
// zero wiring, and a server that never turned events on keeps its old deck
// loader and options exactly.
func (c config) prepareEvents(reg *cards.Registry, load func(string) (host.Deck, error)) (eventsWiring, func(string) (host.Deck, error), error) {
	if !c.events {
		return eventsWiring{}, load, nil
	}
	st, err := eventhost.OpenStore(filepath.Join(c.dir, "events"))
	if err != nil {
		return eventsWiring{}, load, err
	}
	return eventsWiring{store: st, relay: &eventhost.Relay{}}, st.DeckLoader(reg, load), nil
}

// hook routes host.Options.OnMatchEnd through the wiring's relay, keeping
// any hook already set as the relay's Next. A no-op for a zero wiring, so
// a server with -events off keeps its OnMatchEnd untouched.
func (w eventsWiring) hook(o *host.Options) {
	if w.relay == nil {
		return
	}
	w.relay.Next = o.OnMatchEnd
	o.OnMatchEnd = w.relay.OnMatchEnd
	// E3-4: the series hook chains the same way; a Bo1-only relay target
	// never sees a Series table.
	if o.Series == nil {
		o.Series = w.relay.Series
	}
}

// startEvents builds the eventhost — which resumes every stored event —
// and points the wiring's relay at it, so the event games' results recorded
// by host OnMatchEnd reach it. The offered policy set is the server's
// -bot-policies set minus decklist readers (an Env policy never seats in a
// draft event), and the default is -bot-policy unless it reads decklists,
// else bots.Default. Returns nil when -events is off.
func (c config) startEvents(w eventsWiring, r *host.Registry, reg *cards.Registry, gate *seatGate) (*eventhost.Host, error) {
	if w.store == nil {
		return nil, nil
	}
	if gate == nil {
		return nil, fmt.Errorf("gorged: draft events need a seat gate to mint game tokens")
	}
	policies := make([]string, 0, len(c.botPolicies))
	for _, p := range c.botPolicies {
		if !bots.ReadsDecklists(p) {
			policies = append(policies, p)
		}
	}
	def := c.botPolicy
	if bots.ReadsDecklists(def) {
		def = bots.Default
	}
	evh, err := eventhost.New(eventhost.Options{
		Registry:       r,
		Store:          w.store,
		Cards:          reg,
		Policies:       policies,
		DefaultPolicy:  def,
		MaxLiveEvents:  c.maxEvents,
		Mulligans:      c.mulligans,
		AutoMana:       c.autoMana,
		BotAutoPayMana: c.botAutoPayMana,
		BotCommonSense: c.botCommonSense,
		MintSeat: func(t host.TableID, seat int) (string, error) {
			return gate.mint(t, state.PlayerID(seat))
		},
	})
	if err != nil {
		return nil, err
	}
	w.relay.Set(evh.OnMatchEnd)
	w.relay.SetSeries(evh.Series)
	return evh, nil
}
