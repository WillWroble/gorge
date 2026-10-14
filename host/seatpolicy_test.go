package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/protocol"
	"github.com/adams-shaun/gorge/view"
)

// seatPolicyTable is a 2-seat bot-only table the per-seat tests play. It
// turns auto-payment on: without it the hosted manual-mana policies play
// identical games on the sample decks, so a per-seat policy could not be
// told apart from the table default by the match's head.
func seatPolicyTable(id TableID) TableConfig {
	return TableConfig{ID: id, Name: "per-seat " + string(id), Seats: 2, Decks: []string{"a", "b"}, Seed: 7, Spectator: view.Omniscient, AutoMana: true, BotAutoPayMana: true}
}

func playHead(t *testing.T, cfg TableConfig) string {
	t.Helper()
	r, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.AddTable(cfg); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(cfg.ID); err != nil {
		t.Fatal(err)
	}
	r.Wait(cfg.ID)
	ms, err := r.Matches(cfg.ID)
	if err != nil || len(ms) != 1 {
		t.Fatalf("matches %+v, %v", ms, err)
	}
	return ms[0].Head
}

func TestSeatBotPoliciesPlayAMatch(t *testing.T) {
	t.Parallel()
	// Precondition: the two named policies are distinct registered entries,
	// so a finished match here really exercised both per-seat policies.
	e1, ok1 := bots.Lookup("bot")
	e2, ok2 := bots.Lookup("lethal-pressure")
	if !ok1 || !ok2 || e1.Name == e2.Name {
		t.Fatalf("policies not distinct registered entries: bot=%v lethal-pressure=%v", ok1, ok2)
	}
	cfg := seatPolicyTable("t1")
	cfg.SeatBotPolicies = []string{"bot", "lethal-pressure"}
	r, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.AddTable(cfg); err != nil {
		t.Fatal(err)
	}
	if err := r.Start("t1"); err != nil {
		t.Fatal(err)
	}
	r.Wait("t1")
	ms, err := r.Matches("t1")
	if err != nil || len(ms) != 1 {
		t.Fatalf("matches %+v, %v", ms, err)
	}
	m := ms[0]
	if m.State != protocol.MatchFinished || m.Head == "" {
		t.Fatalf("match %+v", m)
	}
	if m.Result != "win" && m.Result != "draw" {
		t.Fatalf("result %q", m.Result)
	}
	// The seat policies must actually reach the seat build: a same-seed
	// table without SeatBotPolicies plays "bot" in both seats, so its head
	// must differ from this one (seat 1 played lethal-pressure).
	defaultHead := playHead(t, seatPolicyTable("t2"))
	if m.Head == defaultHead {
		t.Fatalf("per-seat policies did not change the game: head %q equals the default-policy head", m.Head)
	}
}

func TestSeatBotPoliciesSameAsDefaultIsByteIdentical(t *testing.T) {
	t.Parallel()
	p, err := NormalizeBotPolicy("")
	if err != nil || p == "" {
		t.Fatalf("default policy %q, %v", p, err)
	}
	if _, ok := bots.Lookup(p); !ok {
		t.Fatalf("default policy %q not registered", p)
	}
	plain := seatPolicyTable("t1")
	perSeat := seatPolicyTable("t1")
	perSeat.SeatBotPolicies = []string{p, p}
	h1 := playHead(t, plain)
	h2 := playHead(t, perSeat)
	if h1 == "" || h1 != h2 {
		t.Fatalf("heads differ: default %q per-seat %q", h1, h2)
	}
}

func TestSeatBotPoliciesValidation(t *testing.T) {
	t.Parallel()
	refuse := func(name string, cfg TableConfig, wantSubstr string) {
		t.Helper()
		r, err := New(testOptions(t))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		err = r.AddTable(cfg)
		if err == nil || !strings.Contains(err.Error(), wantSubstr) {
			t.Fatalf("%s: AddTable error = %v, want substring %q", name, err, wantSubstr)
		}
		if got := r.Tables(); len(got) != 0 {
			t.Fatalf("%s: refused table retained: %+v", name, got)
		}
	}
	refuse("unknown", func() TableConfig {
		c := seatPolicyTable("t1")
		c.SeatBotPolicies = []string{"no-such-policy"}
		return c
	}(), "unknown bot policy")
	refuse("env", func() TableConfig {
		c := seatPolicyTable("t1")
		c.SeatBotPolicies = []string{"sb-tactical"}
		return c
	}(), "reads opponents' decklists")
	refuse("human", func() TableConfig {
		c := seatPolicyTable("t1")
		c.Humans = []int{0}
		c.SeatBotPolicies = []string{"bot"}
		return c
	}(), "seat 0 is human")
	refuse("too-long", func() TableConfig {
		c := seatPolicyTable("t1")
		c.SeatBotPolicies = []string{"bot", "bot", "bot"}
		return c
	}(), "3 seat bot policies for 2 seats")

	// An empty entry is accepted and seat 0 falls back to BotPolicy.
	cfg := seatPolicyTable("t1")
	cfg.SeatBotPolicies = []string{"", "bot"}
	r, err := New(testOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.AddTable(cfg); err != nil {
		t.Fatalf("empty-entry slice refused: %v", err)
	}
	if got := r.Tables(); len(got) != 1 || got[0].ID != "t1" {
		t.Fatalf("tables %+v", got)
	}
	r.mu.RLock()
	stored := r.tables["t1"].cfg
	r.mu.RUnlock()
	if fallback := stored.seatBotPolicy(0); fallback != bots.BotPolicy {
		t.Fatalf("seat 0 policy = %q, want fallback %q", fallback, bots.BotPolicy)
	}
	if perSeat := stored.seatBotPolicy(1); perSeat != "bot" {
		t.Fatalf("seat 1 policy = %q, want bot", perSeat)
	}
}

func TestEventTableExemptFromOnDemandLimit(t *testing.T) {
	t.Parallel()
	opts := testOptions(t)
	opts.MaxOnDemandTables = 1
	r, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	first := TableConfig{ID: "g1", Seats: 2, Decks: []string{"a", "b"}, Spectator: view.Public, OnDemand: true}
	if err := r.AddTable(first); err != nil {
		t.Fatal(err)
	}
	second := TableConfig{ID: "g2", Seats: 2, Decks: []string{"a", "b"}, Spectator: view.Public, OnDemand: true}
	if err := r.AddTable(second); err == nil || !strings.Contains(err.Error(), "on-demand table limit 1") {
		t.Fatalf("second private table error = %v, want configured limit", err)
	}
	// An event-owned table is exempt from the limit.
	evt := TableConfig{ID: "e1t", Seats: 2, Decks: []string{"a", "b"}, Spectator: view.Public, OnDemand: true, Event: "e1"}
	if err := r.AddTable(evt); err != nil {
		t.Fatalf("event table refused: %v", err)
	}
	got := r.Tables()
	if len(got) != 2 {
		t.Fatalf("tables %+v, want g1 and e1t", got)
	}
	// TableInfo.Event tells the client the table is the event's (no restart).
	for _, ti := range got {
		if want := ti.ID == "e1t"; ti.Event != want {
			t.Fatalf("table %s Event = %v, want %v", ti.ID, ti.Event, want)
		}
	}
	// Event without OnDemand is refused.
	noOD := TableConfig{ID: "e2", Seats: 2, Decks: []string{"a", "b"}, Spectator: view.Public, Event: "e1"}
	if err := r.AddTable(noOD); err == nil || !strings.Contains(err.Error(), "on-demand") {
		t.Fatalf("event without OnDemand error = %v", err)
	}
	// Event with Perpetual is refused.
	perp := TableConfig{ID: "e3", Seats: 2, Decks: []string{"a", "b"}, Spectator: view.Public, OnDemand: true, Perpetual: true, Event: "e1"}
	if err := r.AddTable(perp); err == nil || !strings.Contains(err.Error(), "perpetual") {
		t.Fatalf("event perpetual error = %v", err)
	}
	if got := r.Tables(); len(got) != 2 {
		t.Fatalf("refused event tables retained: %+v", got)
	}
}

func TestSeatBotPoliciesPersist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := New(diskOptions(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	cfg := TableConfig{ID: "t1", Name: "persisted", Seats: 2, Decks: []string{"a", "b"}, Seed: 3, Spectator: view.Public, SeatBotPolicies: []string{"bot", "lethal-pressure"}}
	if err := r.AddTable(cfg); err != nil {
		r.Close()
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "tables.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Precondition: the field reached the persisted file, so the restart
	// assertion below compares a real value, not an absence.
	if !strings.Contains(string(raw), `"seat_bot_policies": [`) || !strings.Contains(string(raw), `"bot"`) || !strings.Contains(string(raw), `"lethal-pressure"`) {
		t.Fatalf("tables.json does not record the seat bot policies:\n%s", raw)
	}
	r2, err := New(diskOptions(t, dir))
	if err != nil {
		t.Fatalf("restart with persisted seat policies: %v", err)
	}
	defer r2.Close()
	got := r2.Tables()
	if len(got) != 1 || got[0].ID != "t1" {
		t.Fatalf("tables after restart %+v", got)
	}
}
