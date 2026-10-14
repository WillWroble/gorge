package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/bots/azc4"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/host/httpapi"
	"github.com/adams-shaun/gorge/internal/gonet/gonettest"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/view"
)

var azc4Once sync.Once

// enableAZ4 registers az-c4 through gorged's own flag path (a synthetic
// network, a two-deck belief directory), once per test binary.
func enableAZ4(t *testing.T) {
	t.Helper()
	azc4Once.Do(func() {
		dir, err := os.MkdirTemp("", "gorged-azc4-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		path, err := gonettest.Write(dir, "net.dzgw")
		if err != nil {
			t.Fatal(err)
		}
		belief := filepath.Join(dir, "belief")
		if err := os.Mkdir(belief, 0o755); err != nil {
			t.Fatal(err)
		}
		writeDeck(t, belief, "b1", "")
		writeDeck(t, belief, "b2", "")
		c := config{az4Net: path, az4Decks: dir, az4Belief: belief, autoMana: true, botAutoPayMana: true}
		if err := c.enableAZC4(testutil.CorpusRegistry(t)); err != nil {
			t.Fatal(err)
		}
	})
	if _, ok := bots.Lookup(azc4.Policy); !ok {
		t.Fatal("az-c4 is not registered")
	}
}

// The az-c4 flags: the network needs its decks and its belief (closed lists
// are the point), never -allow-decklist-bots; every refusal happens before
// anything registers.
func TestAZC4FlagsValidate(t *testing.T) {
	ok := config{az4Net: "n.dzgw", az4Decks: "d", az4Belief: "b", autoMana: true, botAutoPayMana: true}
	if err := ok.checkAZC4Flags(); err != nil {
		t.Fatalf("a complete configuration without -allow-decklist-bots: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*config)
		want string
	}{
		{"decks without a network", func(c *config) { c.az4Net = "" }, "need -az-c4-net"},
		{"belief without a network", func(c *config) { c.az4Net, c.az4Decks = "", "" }, "need -az-c4-net"},
		{"network without decks", func(c *config) { c.az4Decks = "" }, "needs -az-c4-decks"},
		{"network without a belief", func(c *config) { c.az4Belief = "" }, "needs -az-c4-belief"},
		{"belief pool without a belief", func(c *config) { c.az4Belief, c.az4BeliefPool = "", "pool.tsv" }, "-az-c4-belief-pool needs -az-c4-belief"},
		{"no -auto-mana", func(c *config) { c.autoMana = false }, "-auto-mana"},
		{"no -bot-auto-mana", func(c *config) { c.botAutoPayMana = false }, "-bot-auto-mana"},
	}
	for _, c := range cases {
		cfg := ok
		c.mut(&cfg)
		if err := cfg.checkAZC4Flags(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one containing %q", c.name, err, c.want)
		}
	}
	if err := (config{}).checkAZC4Flags(); err != nil {
		t.Errorf("no az-c4 flags: %v", err)
	}
}

// az-c4 does not read decklists: it is in the default offered set, and the
// vs-bot flow seats a human against it, explicitly or through a deck of its
// pool, dealing both seats from that pool.
func TestCreateGameSeatsAHumanAgainstAZC4(t *testing.T) {
	enableAZ4(t)
	offered, err := botPolicyOffered("")
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(offered, azc4.Policy) {
		t.Fatalf("offered set %v lacks az-c4", offered)
	}
	r, gate := freshGameLock(t)
	c := config{autoMana: true, botAutoPayMana: true, botPolicy: host.BotPolicy,
		policyPools: map[string][]string{azc4.Policy: {"a", "b"}}}
	create := c.createGame(r, gate, nil, []string{"c", "d"}, view.Omniscient)
	for _, req := range []httpapi.CreateGameOptions{
		{Format: host.FormatConstructed, BotPolicy: azc4.Policy},
		{Format: host.FormatConstructed, HumanDeck: "a"},
	} {
		resp, err := create(req)
		if err != nil {
			t.Fatalf("%+v: %v", req, err)
		}
		for _, d := range waitSeatDecks(t, r, resp.Table) {
			if d != "a" && d != "b" {
				t.Fatalf("%+v: dealt %q, outside az-c4's pool", req, d)
			}
		}
	}
	// The pool stays reserved: another policy may not deal from it.
	if _, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: host.BotPolicy, HumanDeck: "a"}); err == nil ||
		!strings.Contains(err.Error(), `reserved for bot policy "az-c4"`) {
		t.Fatalf("an ordinary game took a deck reserved for az-c4: %v", err)
	}
}

// A deck directory shared by az-c3 and az-c4 is one pool reserved for both:
// catalogued once, picked for az-c4 by a request that names no policy (az-c3
// cannot seat a human), still refused for an explicit az-c3 pick.
func TestAZC3AndAZC4ShareADeckDirectory(t *testing.T) {
	enableAZ(t)
	enableAZ4(t)
	reg := testutil.CorpusRegistry(t)
	dir := t.TempDir()
	writeDeck(t, dir, "FDN_a", "")
	writeDeck(t, dir, "FDN_b", "")
	c := config{az3Decks: dir, az4Decks: dir + string(filepath.Separator)}
	az3, cat3, err := c.azc3Pool(reg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	az4, cat4, cat3, err := c.azc4Pool(reg, nil, nil, az3, cat3)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(az3, az4) || len(cat4) != 0 || len(cat3) != 2 {
		t.Fatalf("pools %v / %v, catalogue %d + %d: want one shared pool catalogued once", az3, az4, len(cat3), len(cat4))
	}
	for _, d := range cat3 {
		if !httpapi.PoolHas(d.Pool, azc3.Policy) || !httpapi.PoolHas(d.Pool, azc4.Policy) {
			t.Errorf("deck %s pool %q: want both policies", d.ID, d.Pool)
		}
	}
	c.policyPools = map[string][]string{azc3.Policy: az3, azc4.Policy: az4}
	if got := c.reservedPick("FDN_a"); got != azc4.Policy {
		t.Errorf("a shared deck picks %q, want az-c4", got)
	}
	if got := c.reservedFor("FDN_a"); got != "az-c3, az-c4" {
		t.Errorf("reservedFor %q", got)
	}
	// Two directories that both hold an id are ambiguous: refused.
	other := t.TempDir()
	writeDeck(t, other, "FDN_a", "")
	c2 := config{az3Decks: dir, az4Decks: other}
	if _, _, _, err := c2.azc4Pool(reg, nil, nil, az3, cat3); err == nil || !strings.Contains(err.Error(), "-az-c3-decks") {
		t.Errorf("an id in both directories: error %v", err)
	}
	// The loader resolves a shared id from the shared directory.
	load := reservedLoader(reg, deckLoaders(reg, t.TempDir(), dir, az3), dir, az4)
	if d, err := load("FDN_b"); err != nil || len(d.Cards) != 40 {
		t.Errorf("load(FDN_b) = %d cards, %v", len(d.Cards), err)
	}
	// Draft events never offer a policy with a reserved pool.
	if got := c.reservedPolicyNames(); !reflect.DeepEqual(got, []string{azc3.Policy, azc4.Policy}) {
		t.Errorf("reserved policies %v", got)
	}
}

// The az-c4 demo table is a bot-only table that needs no decklist opt-in.
func TestAZC4DemoTable(t *testing.T) {
	c := config{az4Net: "n", az4DemoTable: true, autoMana: true, botAutoPayMana: true}
	tc := c.azc4Table("t3", 7, []string{"a", "b"}, view.Public)
	if tc == nil || tc.BotPolicy != azc4.Policy || tc.AllowDecklistBots || len(tc.Humans) != 0 || tc.ID != "t3" {
		t.Fatalf("demo table %+v", tc)
	}
	c.az4DemoTable = false
	if c.azc4Table("t3", 7, []string{"a"}, view.Public) != nil {
		t.Fatal("-az-c4-demo-table=false still built the table")
	}
}
