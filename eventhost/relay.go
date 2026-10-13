package eventhost

import (
	"sync/atomic"

	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/protocol"
)

// Relay lets gorged hand host.Options.OnMatchEnd to an eventhost that can
// only be built after the Registry: the Registry wants the relay in its
// Options, the Host wants the Registry in its own. Next, when set, runs
// first; the target the Host installs with Set runs after it.
type Relay struct {
	Next   host.OnMatchEndFunc
	target atomic.Pointer[host.OnMatchEndFunc]
}

// Set installs the Host's OnMatchEnd as the relay's target.
func (r *Relay) Set(fn host.OnMatchEndFunc) { r.target.Store(&fn) }

// OnMatchEnd chains Next and the target, returning the first non-nil error.
func (r *Relay) OnMatchEnd(t host.TableID, k int, m protocol.MatchInfo) error {
	if r.Next != nil {
		if err := r.Next(t, k, m); err != nil {
			return err
		}
	}
	if fn := r.target.Load(); fn != nil {
		return (*fn)(t, k, m)
	}
	return nil
}
