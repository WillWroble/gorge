//go:build !lab

package main

import (
	"context"
	"errors"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/server/host"
	"github.com/adams-shaun/gorge/server/host/httpapi"
)

// Draft events are a lab extension (lab/draft; docs/decisions/0001-repo-layout.md).
// A gorged built without -tags lab links none of it: -events is refused at
// startup and /api/events is not mounted. events.go and art_sets.go are the
// lab build's real wiring; `make deploy-demo` builds with -tags lab.

// eventRuntime is what serve holds of the draft-event runtime: the
// /api/events backend, closed before the registry on shutdown. The lab
// build's eventhost.Host is the only implementation.
type eventRuntime interface {
	httpapi.EventService
	Close() error
}

var errEventsNeedLab = errors.New("gorged: -events needs a binary built with -tags lab (draft events live in lab/draft)")

type eventsWiring struct{}

func (c config) prepareEvents(_ *cards.Registry, load func(string) (host.Deck, error)) (eventsWiring, func(string) (host.Deck, error), error) {
	if c.events {
		return eventsWiring{}, load, errEventsNeedLab
	}
	return eventsWiring{}, load, nil
}

func (eventsWiring) hook(*host.Options) {}

func (c config) startEvents(eventsWiring, *host.Registry, *cards.Registry, *seatGate) (eventRuntime, error) {
	return nil, nil
}

func prewarmSetArt(context.Context, *artCache, func(string, ...any)) {}
