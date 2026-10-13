package main

// Operator ruling 2026-10-13: a bot policy that reads every seat's real
// decklist (bots.Info.Env) is never offered on gorged's public
// /api/bot-policies list and is never the vs-bot default, because every
// request the offered set feeds always seats a human. The server's own
// bot-only tables (today only the az-c3 demo table) may run one behind the
// explicit -allow-decklist-bots opt-in; az-c3 cannot be served without it
// and is unreachable from the vs-bot flow either way.

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/host/httpapi"
	"github.com/adams-shaun/gorge/internal/gonet/gonettest"
	"github.com/adams-shaun/gorge/seat"
	"github.com/adams-shaun/gorge/view"
)

// formatSpyPolicy is a test-only NON-decklist constructed-only entry, so
// botpolicies_test's "default lacks a format" refusal stays testable: the
// ruling of 2026-10-13 refuses a decklist-reading default before the format
// check runs, and every real non-Env entry plays both formats.
const formatSpyPolicy = "dlb-format-spy"

var formatSpyOnce sync.Once

func registerFormatSpy(t *testing.T) {
	t.Helper()
	formatSpyOnce.Do(func() {
		bots.Register(bots.Entry{Info: bots.Info{
			Name: formatSpyPolicy, Label: "test-only constructed-only", Tier: bots.Experimental,
			Description: "test-only entry for the default-format refusal",
			Formats:     []string{"constructed"}, MaxSeats: 2,
		}, New: func(o bots.Options) (seat.Seat, error) { return seat.NewBot(o.Seed), nil }})
	})
	e, ok := bots.Lookup(formatSpyPolicy)
	if !ok {
		t.Fatal("the format-spy entry was not registered")
	}
	// Precondition: it is NOT a decklist reader (or the Env refusal would
	// fire first and the format case would be vacuous) and it lacks
	// commander.
	if bots.ReadsDecklists(formatSpyPolicy) {
		t.Fatal("the format-spy entry unexpectedly reads decklists")
	}
	if contains(e.Formats, host.FormatCommander.String()) {
		t.Fatalf("the format-spy entry unexpectedly supports commander (formats %v)", e.Formats)
	}
}

// nonDecklistPolicyNames is the offered set an empty -bot-policies resolves
// to: every registered entry that does not read opponents' decklists, in
// registry order.
func nonDecklistPolicyNames() []string {
	var out []string
	for _, name := range bots.Names() {
		if !bots.ReadsDecklists(name) {
			out = append(out, name)
		}
	}
	return out
}

func TestBotPoliciesDefaultExcludesDecklistBots(t *testing.T) {
	offered, err := botPolicyOffered("")
	if err != nil {
		t.Fatalf("botPolicyOffered(\"\"): %v", err)
	}
	// Precondition: the set is non-empty and carries the server's real
	// defaults, so "excludes" is not trivially true of an empty list.
	for _, want := range []string{bots.Default, host.LethalPressurePolicy} {
		if !containsString(offered, want) {
			t.Fatalf("offered set %v lost the default %q", offered, want)
		}
	}
	var excluded []string
	for _, name := range bots.Names() {
		if !bots.ReadsDecklists(name) {
			if !containsString(offered, name) {
				t.Errorf("non-decklist policy %q missing from the offered set", name)
			}
			continue
		}
		excluded = append(excluded, name)
		if containsString(offered, name) {
			t.Errorf("decklist-reading policy %q is offered on a server where vs-bot seats a human", name)
		}
	}
	if len(excluded) == 0 {
		t.Fatal("no registered policy reads decklists; the test's exclusion is vacuous")
	}
	// The wire listing gorged arms at startup shows the same set.
	list := buildBotPolicyList(bots.Default, offered)
	for _, p := range list.Policies {
		if bots.ReadsDecklists(p.Name) {
			t.Errorf("/api/bot-policies lists decklist-reading policy %q", p.Name)
		}
	}
}

func TestBotPoliciesNamingDecklistBotIsAnError(t *testing.T) {
	var envNames []string
	for _, name := range bots.Names() {
		if bots.ReadsDecklists(name) {
			envNames = append(envNames, name)
		}
	}
	if len(envNames) == 0 {
		t.Fatal("no registered policy reads decklists; the test would be vacuous")
	}
	for _, name := range envNames {
		_, err := botPolicyOffered("bot," + name)
		if err == nil {
			t.Errorf("-bot-policies accepted decklist-reading policy %q", name)
		} else if !strings.Contains(err.Error(), "reads opponents' decklists") {
			t.Errorf("%q: error %v, want one naming the reason", name, err)
		}
		// A decklist-reading default is refused the same way: it is what an
		// omitted-field vs-bot request plays against, and that request
		// always seats a human.
		_, _, err = validateBotPolicyFlags(name, "")
		if err == nil {
			t.Errorf("-bot-policy %q accepted as the default", name)
		} else if !strings.Contains(err.Error(), "reads opponents' decklists") {
			t.Errorf("%q default: error %v, want one naming the reason", name, err)
		}
	}
}

func TestAZC3NeedsAllowDecklistBots(t *testing.T) {
	dir, err := os.MkdirTemp("", "gorged-dlb-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path, err := gonettest.Write(dir, "net.dzgw")
	if err != nil {
		t.Fatal(err)
	}
	// Precondition: az-c3 is registered (this test or azc3_test's enableAZ
	// enabled it over the synthetic network).
	if _, ok := bots.Lookup(azc3.Policy); !ok {
		if err := (config{az3Net: path, az3Decks: dir, autoMana: true, botAutoPayMana: true, allowDecklistBots: true}).enableAZC3(); err != nil {
			t.Fatalf("with -allow-decklist-bots the flags should pass and register: %v", err)
		}
	}
	if _, ok := bots.Lookup(azc3.Policy); !ok {
		t.Fatal("az-c3 is not registered")
	}
	// Without the opt-in the flags are refused, naming the rule, even though
	// the policy is registered: serving az-c3 requires -allow-decklist-bots.
	err = (config{az3Net: path, az3Decks: dir, autoMana: true, botAutoPayMana: true}).enableAZC3()
	if err == nil {
		t.Fatal("enableAZC3 accepted -az-c3-net without -allow-decklist-bots")
	}
	if !strings.Contains(err.Error(), "-allow-decklist-bots") {
		t.Fatalf("error %v, want one naming -allow-decklist-bots", err)
	}
}

func TestAZC3NotReachableFromVsBot(t *testing.T) {
	enableAZ(t)
	// The opt-in itself is set, to prove the refusal is not just the missing
	// flag: the vs-bot flow seats a HUMAN at seat 0, and no opt-in buys that.
	c := azConfig()
	c.allowDecklistBots = true
	r, gate := freshGameLock(t)
	create := c.createGame(r, gate, nil, []string{"c", "d"}, view.Omniscient)
	for _, tc := range []struct {
		name string
		req  httpapi.CreateGameOptions
	}{
		{"explicit pick", httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: azc3.Policy}},
		// An FDN deck reserved for az-c3 used to pick the policy; now it
		// must refuse rather than seat a human against it.
		{"reserved-deck pick", httpapi.CreateGameOptions{Format: host.FormatConstructed, HumanDeck: "a"}},
	} {
		_, err := create(tc.req)
		if err == nil {
			t.Errorf("%s: the vs-bot flow seated az-c3", tc.name)
		} else if !strings.Contains(err.Error(), "reads opponents' decklists") {
			t.Errorf("%s: error %v, want the decklist refusal", tc.name, err)
		}
	}
	if n := len(r.Tables()); n != 0 {
		t.Errorf("refused requests created %d table(s)", n)
	}
}
