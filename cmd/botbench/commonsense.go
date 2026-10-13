package main

import (
	"strings"

	"github.com/adams-shaun/gorge/bots/sanity"
	"github.com/adams-shaun/gorge/seat"
)

// sanitySuffix names the guarded variant of a built-in policy: `bot+sanity`
// is `bot` behind bots/sanity.Wrap. The guard drops hostile removal the
// engine's target hints say can only hit the seat's own side (see
// bots/sanity). It is opt-in: no plain name changes.
const sanitySuffix = "+sanity"

// sanityBases are the built-in policies the guard may decorate: the default
// bot (manual and auto-pay mana) and sb-tactical, nothing else.
var sanityBases = []string{"bot", "bot-auto-pay", "sb-tactical"}

// plainPolicies holds the undecorated constructors captured before either
// the +sanity entries or -common-sense replaced anything.
var plainPolicies = map[string]func(seed uint64) seat.Seat{}

// registerSanityPolicies adds `<base>+sanity` for each sanity base. Called at
// the end of init, after every base is bound.
func registerSanityPolicies() {
	for _, base := range sanityBases {
		c, ok := policies[base]
		if !ok {
			continue
		}
		plainPolicies[base] = c
		policies[base+sanitySuffix] = func(seed uint64) seat.Seat { return sanity.Wrap(c(seed)) }
	}
}

// applyCommonSense is the -common-sense flag: every plain sanity base is
// rebound to its guarded form, so a run that names `bot` or `sb-tactical`
// plays them behind the guard on both sides.
func applyCommonSense() {
	for _, base := range sanityBases {
		if c, ok := plainPolicies[base]; ok {
			policies[base] = func(seed uint64) seat.Seat { return sanity.Wrap(c(seed)) }
		}
	}
}

func basePolicyName(name string) string { return strings.TrimSuffix(name, sanitySuffix) }
