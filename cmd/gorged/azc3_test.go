package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/azc3"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/host/httpapi"
	"github.com/adams-shaun/gorge/internal/gonet/gonettest"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/view"
)

var azc3Once sync.Once

// enableAZ registers az-c3 over a synthetic network, once per test binary, and
// returns the path it loaded.
func enableAZ(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gorged-azc3-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path, err := gonettest.Write(dir, "net.dzgw")
	if err != nil {
		t.Fatal(err)
	}
	azc3Once.Do(func() {
		if err := (config{az3Net: path, az3Decks: dir, vsbot: true, autoMana: true, botAutoPayMana: true}).enableAZC3(); err != nil {
			t.Fatal(err)
		}
	})
	if _, ok := bots.Lookup(azc3.Policy); !ok {
		t.Fatal("az-c3 is not registered")
	}
	return path
}

// The flags come as a pair and need what az-c3 needs; every refusal happens
// before anything registers.
func TestAZC3FlagsValidate(t *testing.T) {
	ok := config{az3Net: "n.dzgw", az3Decks: "d", vsbot: true, autoMana: true, botAutoPayMana: true}
	cases := []struct {
		name string
		mut  func(*config)
		want string
	}{
		{"decks without a network", func(c *config) { c.az3Net = "" }, "-az-c3-decks needs -az-c3-net"},
		{"network without decks", func(c *config) { c.az3Decks = "" }, "needs -az-c3-decks"},
		{"no -vsbot", func(c *config) { c.vsbot = false }, "needs -vsbot"},
		{"no -auto-mana", func(c *config) { c.autoMana = false }, "-auto-mana"},
		{"no -bot-auto-mana", func(c *config) { c.botAutoPayMana = false }, "-bot-auto-mana"},
	}
	for _, c := range cases {
		cfg := ok
		c.mut(&cfg)
		err := cfg.enableAZC3()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one containing %q", c.name, err, c.want)
		}
	}
	if _, found := bots.Lookup(azc3.Policy); found {
		t.Error("a refused configuration left az-c3 registered")
	}
	if err := (config{}).enableAZC3(); err != nil {
		t.Errorf("no az-c3 flags: %v", err)
	}
	if _, found := bots.Lookup(azc3.Policy); found {
		t.Error("a server that never asked for az-c3 has it registered")
	}
	// A network the loader refuses is a startup error, not a half-served policy.
	bad := filepath.Join(t.TempDir(), "bad.dzgw")
	if err := os.WriteFile(bad, []byte("not a network"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := ok
	cfg.az3Net = bad
	if err := cfg.enableAZC3(); err == nil {
		t.Error("a bad network file was accepted")
	}
	if _, found := bots.Lookup(azc3.Policy); found {
		t.Error("a bad network left az-c3 registered")
	}
}

func azConfig() config {
	return config{mulligans: 0, autoMana: true, botAutoPayMana: true,
		policyPools: map[string][]string{azc3.Policy: {"a", "b"}}}
}

// A game against az-c3 deals from its pool for BOTH seats, and from nothing
// else; an ordinary game never deals a reserved deck.
func TestCreateGameKeepsAZC3ToItsPool(t *testing.T) {
	enableAZ(t)
	r, gate := freshGameLock(t)
	create := azConfig().createGame(r, gate, nil, []string{"c", "d"}, view.Omniscient)

	for i := 0; i < 6; i++ {
		resp, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: azc3.Policy})
		if err != nil {
			t.Fatalf("game %d: %v", i, err)
		}
		ms := waitSeatDecks(t, r, resp.Table)
		for _, d := range ms {
			if d != "a" && d != "b" {
				t.Fatalf("an az-c3 game dealt %q, outside its pool {a,b}", d)
			}
		}
	}
	for i := 0; i < 6; i++ {
		resp, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: host.BotPolicy})
		if err != nil {
			t.Fatalf("ordinary game %d: %v", i, err)
		}
		for _, d := range waitSeatDecks(t, r, resp.Table) {
			if d != "c" && d != "d" {
				t.Fatalf("an ordinary game dealt %q, a deck reserved for az-c3", d)
			}
		}
	}
}

func TestCreateGameRefusesDecksOutsideThePolicysPool(t *testing.T) {
	enableAZ(t)
	r, gate := freshGameLock(t)
	create := azConfig().createGame(r, gate, nil, []string{"c", "d"}, view.Omniscient)
	for _, c := range []struct {
		name string
		req  httpapi.CreateGameOptions
		want string
	}{
		{"human deck outside the pool", httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: azc3.Policy, HumanDeck: "c"}, "human deck \"c\" is not in the az-c3 pool"},
		{"bot deck outside the pool", httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: azc3.Policy, BotDeck: "d"}, "bot deck \"d\" is not in the az-c3 pool"},
		{"reserved deck, other policy", httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: host.BotPolicy, HumanDeck: "a"}, "reserved for bot policy \"az-c3\""},
		{"reserved bot deck, other policy", httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: host.BotPolicy, BotDeck: "b"}, "reserved for bot policy \"az-c3\""},
	} {
		if _, err := create(c.req); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one containing %q", c.name, err, c.want)
		}
	}
	if n := len(r.Tables()); n != 0 {
		t.Errorf("refused requests created %d table(s)", n)
	}
	// Explicit picks inside the pool are honoured.
	resp, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: azc3.Policy, HumanDeck: "a", BotDeck: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitSeatDecks(t, r, resp.Table); !reflect.DeepEqual(got, []string{"a", "b"}) && !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Errorf("explicit pool picks dealt %v", got)
	}
}

// A server whose tables lack payment plans cannot build az-c3: the request is
// refused, and no match is started to crash on its first seat.
func TestCreateGameRefusesAZC3WithoutAutoPay(t *testing.T) {
	enableAZ(t)
	for _, mut := range []func(*config){func(c *config) { c.autoMana = false }, func(c *config) { c.botAutoPayMana = false }} {
		r, gate := freshGameLock(t)
		c := azConfig()
		mut(&c)
		create := c.createGame(r, gate, nil, []string{"c", "d"}, view.Omniscient)
		_, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed, BotPolicy: azc3.Policy})
		if err == nil || !strings.Contains(err.Error(), "automatic mana payment") {
			t.Errorf("error %v, want the automatic-payment refusal", err)
		}
		if n := len(r.Tables()); n != 0 {
			t.Errorf("refused request created %d table(s)", n)
		}
	}
}

// waitSeatDecks returns the decks dealt to a table's two seats once its match
// is visible.
func waitSeatDecks(t *testing.T, r *host.Registry, table string) []string {
	t.Helper()
	for i := 0; i < 20000; i++ {
		ms, err := r.Matches(host.TableID(table))
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) == 1 && len(ms[0].Seats) == 2 {
			return []string{ms[0].Seats[0].Deck, ms[0].Seats[1].Deck}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no match appeared on table %s", table)
	return nil
}

func writeDeck(t *testing.T, dir, id, extra string) {
	t.Helper()
	body := `{"name":"` + id + `","format":"limited","archetype":"test",` + extra +
		`"cards":[{"name":"Forest","count":20},{"name":"Mountain","count":20}]}`
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// -az-c3-decks is loaded and tagged, and refused when it would let az-c3 play
// something it was not trained on or make a deck id ambiguous.
func TestAZC3PoolLoading(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	c := config{}
	t.Run("no flag, no pool", func(t *testing.T) {
		ids, cat, err := c.azc3Pool(reg, nil, nil)
		if ids != nil || cat != nil || err != nil {
			t.Errorf("got %v %v %v", ids, cat, err)
		}
	})
	t.Run("loaded, sorted and tagged", func(t *testing.T) {
		dir := t.TempDir()
		writeDeck(t, dir, "FDN_b", "")
		writeDeck(t, dir, "FDN_a", "")
		// A 17lands deck carries no archetype: it gets the label, so the picker
		// shows what it is. A deck that has one keeps it.
		raw, err := os.ReadFile(filepath.Join(dir, "FDN_a.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "FDN_a.json"), []byte(strings.Replace(string(raw), `"archetype":"test",`, "", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		c := config{az3Decks: dir}
		ids, cat, err := c.azc3Pool(reg, nil, []string{"other"})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ids, []string{"FDN_a", "FDN_b"}) {
			t.Errorf("pool %v", ids)
		}
		if len(cat) != 2 || cat[0].Pool != azc3.Policy || cat[1].Pool != azc3.Policy || cat[0].Format != "constructed" ||
			!strings.Contains(cat[0].Archetype, "az-c3") || cat[1].Archetype != "test" {
			t.Errorf("catalogue %+v", cat)
		}
	})
	t.Run("an id that shadows -decks", func(t *testing.T) {
		dir := t.TempDir()
		writeDeck(t, dir, "burn", "")
		c := config{az3Decks: dir}
		if _, _, err := c.azc3Pool(reg, nil, []string{"burn"}); err == nil || !strings.Contains(err.Error(), "same id") {
			t.Errorf("error %v", err)
		}
	})
	t.Run("an empty directory", func(t *testing.T) {
		c := config{az3Decks: t.TempDir()}
		if _, _, err := c.azc3Pool(reg, nil, nil); err == nil {
			t.Error("an empty pool was accepted")
		}
	})
	t.Run("a commander deck", func(t *testing.T) {
		dir := t.TempDir()
		writeDeck(t, dir, "cmd", `"commander":"Forest",`)
		c := config{az3Decks: dir}
		if _, _, err := c.azc3Pool(reg, nil, nil); err == nil {
			t.Error("a commander deck was accepted")
		}
	})
	t.Run("the loader resolves pool ids from their own directory", func(t *testing.T) {
		mainDir, azDir := t.TempDir(), t.TempDir()
		writeDeck(t, mainDir, "ordinary", "")
		writeDeck(t, azDir, "FDN_a", "")
		load := deckLoaders(reg, mainDir, azDir, []string{"FDN_a"})
		for _, id := range []string{"ordinary", "FDN_a"} {
			if d, err := load(id); err != nil || len(d.Cards) != 40 {
				t.Errorf("load(%q) = %d cards, %v", id, len(d.Cards), err)
			}
		}
		if _, err := load("absent"); err == nil {
			t.Error("an unknown id loaded")
		}
	})
}

// The web client names no bot policy, so a deck reserved for one picks it:
// choose an FDN deck and the opponent is az-c3, dealt from the same pool.
func TestCreateGameChoosesAZC3FromAPooledDeck(t *testing.T) {
	enableAZ(t)
	r, gate := freshGameLock(t)
	c := azConfig()
	c.botPolicy = host.BotPolicy
	create := c.createGame(r, gate, nil, []string{"c", "d"}, view.Omniscient)

	for _, req := range []httpapi.CreateGameOptions{
		{Format: host.FormatConstructed, HumanDeck: "a"},
		{Format: host.FormatConstructed, BotDeck: "b"},
		{Format: host.FormatConstructed, HumanDeck: "b", BotDeck: "a"},
	} {
		resp, err := create(req)
		if err != nil {
			t.Fatalf("%+v: %v", req, err)
		}
		if resp.BotPolicy != azc3.Policy {
			t.Errorf("%+v: opponent %q, want az-c3", req, resp.BotPolicy)
		}
		for _, d := range waitSeatDecks(t, r, resp.Table) {
			if d != "a" && d != "b" {
				t.Errorf("%+v dealt %q, outside the pool", req, d)
			}
		}
	}
	// No decks named: the server's own default, from the ordinary pool.
	resp, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed})
	if err != nil {
		t.Fatal(err)
	}
	if resp.BotPolicy != host.BotPolicy {
		t.Errorf("no decks named: opponent %q, want the server default %q", resp.BotPolicy, host.BotPolicy)
	}
	// A pooled deck beside an ordinary one is refused, naming the pool.
	if _, err := create(httpapi.CreateGameOptions{Format: host.FormatConstructed, HumanDeck: "a", BotDeck: "c"}); err == nil ||
		!strings.Contains(err.Error(), "not in the az-c3 pool") {
		t.Errorf("mixed pooled and ordinary decks: error %v", err)
	}
}

func TestAZC3DemoTable(t *testing.T) {
	pool := []string{"FDN_a", "FDN_b"}
	c := config{az3Net: "n", az3DemoTable: true, tables: 4, seed: 10, perpetual: true, autoMana: true, botAutoPayMana: true, mulligans: 2}
	got := c.azc3Table(pool, view.Public)
	if got == nil {
		t.Fatal("no demo table")
	}
	if got.ID != "t5" || got.Seats != 2 || got.BotPolicy != azc3.Policy || got.Format != host.FormatConstructed ||
		!reflect.DeepEqual(got.Decks, pool) || len(got.Humans) != 0 || !got.Perpetual || !got.AutoMana || !got.BotAutoPayMana {
		t.Errorf("demo table %+v", got)
	}
	for name, mut := range map[string]func(*config){
		"switched off": func(c *config) { c.az3DemoTable = false },
		"no network":   func(c *config) { c.az3Net = "" },
	} {
		cc := c
		mut(&cc)
		if cc.azc3Table(pool, view.Public) != nil {
			t.Errorf("%s: a demo table was still built", name)
		}
	}
	if c.azc3Table(nil, view.Public) != nil {
		t.Error("an empty pool built a demo table")
	}
}
