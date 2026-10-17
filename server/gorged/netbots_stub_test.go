package gorged

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/searchprobe"
	"github.com/adams-shaun/gorge/engine/seat"
)

// The az net policies' implementations live outside gorge (four-yay links
// them in through RegisterNetBot). gorged's own tests exercise its generic
// machinery -- flags, reserved pools, create-game refusals, demo tables,
// beliefs -- over stub net bots that register the same gorged-relevant
// bots.Info as the real policies and play the heuristic bot's seat.

// stubNetMagic opens every stub network file; anything else is refused, as
// the real policies refuse a file that is not a valid .dzgw network.
const stubNetMagic = "STUBNET"

// writeStubNet writes a stub network file named name in dir.
func writeStubNet(dir, name string) (string, error) {
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, []byte(stubNetMagic+"\n"), 0o644)
}

// stubNetBot is one stub policy: its registry Info and the belief its
// Enable/SetBelief were last given.
type stubNetBot struct {
	info    bots.Info
	mu      sync.Mutex
	enabled bool
	belief  *searchprobe.Belief
}

func (s *stubNetBot) enable(path string, closed *searchprobe.Belief) error {
	name := s.info.Name
	if s.info.ClosedLists && closed == nil {
		return fmt.Errorf("%s: no belief: the opponents' lists must be closed", name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !bytes.HasPrefix(raw, []byte(stubNetMagic)) {
		return fmt.Errorf("%s: %s is not a network", name, path)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.enabled {
		return fmt.Errorf("%s: already enabled", name)
	}
	s.belief = closed
	bots.Register(bots.Entry{Info: s.info, New: s.newSeat})
	s.enabled = true
	return nil
}

func (s *stubNetBot) setBelief(b *searchprobe.Belief) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.belief = b
}

// newSeat refuses a table without payment plans as the real factory does
// and otherwise plays the heuristic bot's seat.
func (s *stubNetBot) newSeat(o bots.Options) (seat.Seat, error) {
	if !o.AutoPayMana {
		return nil, errors.New(s.info.Name + " needs automatic mana payment")
	}
	return bots.New(bots.Default, o)
}

// stubInfo is the gorged-relevant part of the real policy's bots.Info.
func stubInfo(policy string, closed bool) bots.Info {
	return bots.Info{
		Name: policy, Label: policy + " (test stub)", Description: "stub net bot for gorged's tests",
		Tier: bots.Experimental,
		Env:  true, Search: true, ClosedLists: closed,
		Formats:      []string{"constructed"},
		MaxSeats:     2,
		Caretaker:    "bot",
		NeedsAutoPay: true,
	}
}

var stubNetBots = map[string]*stubNetBot{
	policyAZC3: {info: stubInfo(policyAZC3, false)},
	policyAZC4: {info: stubInfo(policyAZC4, true)},
	policyAZC5: {info: stubInfo(policyAZC5, true)},
}

func init() {
	RegisterNetBot(policyAZC3, NetBot{Enable: stubNetBots[policyAZC3].enable, SetBelief: stubNetBots[policyAZC3].setBelief})
	RegisterNetBot(policyAZC4, NetBot{Enable: stubNetBots[policyAZC4].enable})
	RegisterNetBot(policyAZC5, NetBot{Enable: stubNetBots[policyAZC5].enable})
}
