package gorged

import (
	"fmt"
	"sync"

	"github.com/adams-shaun/gorge/bots/searchprobe"
)

// The hosted net policies gorged serves. Their names, flags, deck pools and
// demo tables are gorged's (azc3.go, azclosed.go); their implementation is
// linked in by the binary through RegisterNetBot.
const (
	policyAZC3 = "az-c3"
	policyAZC4 = "az-c4"
	policyAZC5 = "az-c5"
)

// NetBot is a hosted policy whose seat loads a trained network at startup
// (az-c3/az-c4/az-c5, linked in by four-yay's gorged). gorged owns the
// policy names, their flags and the closed deck-list machinery; the binary
// supplies the implementation with RegisterNetBot before Main.
type NetBot struct {
	// Enable loads the network at path and registers the policy in the bots
	// registry. closed is the opponents' deck-list belief for a closed-list
	// policy (az-c4, az-c5) and nil for an open one (az-c3).
	Enable func(path string, closed *searchprobe.Belief) error
	// SetBelief, when non-nil, closes an open policy's opponent lists after
	// Enable (az-c3's -az-c3-belief).
	SetBelief func(*searchprobe.Belief)
}

var (
	netBotsMu sync.Mutex
	netBots   = map[string]NetBot{}
)

// RegisterNetBot links policy's implementation into this binary. Call it
// before Main (from an init or main). policy must be one gorged serves
// (az-c3, az-c4, az-c5) and nb.Enable non-nil; registering a policy twice
// panics.
func RegisterNetBot(policy string, nb NetBot) {
	switch policy {
	case policyAZC3, policyAZC4, policyAZC5:
	default:
		panic(fmt.Sprintf("gorged: RegisterNetBot: %q is not a net bot policy gorged serves (%s, %s, %s)", policy, policyAZC3, policyAZC4, policyAZC5))
	}
	if nb.Enable == nil {
		panic("gorged: RegisterNetBot: " + policy + " has no Enable")
	}
	netBotsMu.Lock()
	defer netBotsMu.Unlock()
	if _, dup := netBots[policy]; dup {
		panic("gorged: RegisterNetBot: " + policy + " registered twice")
	}
	netBots[policy] = nb
}

// netBot returns policy's linked implementation.
func netBot(policy string) (NetBot, bool) {
	netBotsMu.Lock()
	defer netBotsMu.Unlock()
	nb, ok := netBots[policy]
	return nb, ok
}

// notLinked is the startup refusal for a net flag naming a policy this
// binary does not carry.
func notLinked(flag, policy string) error {
	return fmt.Errorf("%s: %s is not linked into this binary (build four-yay's gorged)", flag, policy)
}
