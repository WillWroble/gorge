package host

// HD-3 acceptance tests (docs/superpowers/specs/2026-10-10-human-divergence-design.md
// §5.3-§5.5, ticket HD-3): SnapshotMatch names match k; a shadow built the
// way the host's dispatch builds a seat's input reproduces itself exactly
// (parity, zero divergences); a seat that deviates at chosen intents is
// found at exactly those intents; caretaker and auto intents are never
// scanned.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/adams-shaun/gorge/bots/heuristic"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/bots/sbtactical"
	"github.com/adams-shaun/gorge/bots/searchseat"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/seat"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/server/divergence"
	"github.com/adams-shaun/gorge/server/protocol"
)

// shadowTestSeat plays slot 0 the way the shadow scan assumes a seat plays:
// at every decision it builds a FRESH bots.New(policy, ShadowSeed(matchSeed,
// slot, n)) where n is the decision's intent index (the intents already
// logged), and answers through the same dispatch order the host uses for it
// (Env, then Board, then View). For an Env decision the honest root is dealt
// from RootSeed(shadowSeed, seq), as shadow.Answer does, because the host
// builds roots from the per-seat seed instead. deviate, when set, may swap
// the shadow-equal answer for another one (the injected-divergence test).
// The seat runs on the match goroutine, so reading the live engine's log is
// race-free.
type shadowTestSeat struct {
	policy    string
	deps      bots.Deps
	matchSeed uint64
	slot      state.PlayerID
	live      func() *rules.Engine
	probe     seat.Seat
	deviate   func(d *decision.Decision, n int, b decision.Intent) (decision.Intent, bool)

	seen    []int // intent index of every decision answered
	envs    int   // decisions answered through the Env path
	multi   int   // decisions that offered more than one option
	refused int   // Env decisions whose honest root was refused
}

func newShadowTestSeat(t *testing.T, policy string, deps bots.Deps, matchSeed uint64, live func() *rules.Engine) *shadowTestSeat {
	t.Helper()
	probe, err := bots.New(policy, bots.Options{Seed: 1, Deps: deps})
	if err != nil {
		t.Fatalf("bots.New(%q): %v", policy, err)
	}
	return &shadowTestSeat{policy: policy, deps: deps, matchSeed: matchSeed, live: live, probe: probe}
}

func (s *shadowTestSeat) index() (int, uint64) {
	n := len(s.live().L.Intents)
	return n, divergence.ShadowSeed(s.matchSeed, s.slot, n)
}

func (s *shadowTestSeat) fresh(seed uint64) seat.Seat {
	in, err := bots.New(s.policy, bots.Options{Seed: seed, Deps: s.deps})
	if err != nil {
		panic(err)
	}
	return in
}

func (s *shadowTestSeat) finish(d decision.Decision, n int, in decision.Intent, err error) (decision.Intent, error) {
	s.seen = append(s.seen, n)
	if len(d.Options) > 1 {
		s.multi++
	}
	if err == nil && s.deviate != nil {
		if alt, ok := s.deviate(&d, n, in); ok {
			return alt, nil
		}
	}
	return in, err
}

func (s *shadowTestSeat) WantsPaymentActions() bool {
	c, ok := s.probe.(seat.PaymentPlanConsumer)
	return ok && c.WantsPaymentActions()
}

func (s *shadowTestSeat) WantsEnv(d *decision.Decision) bool {
	es, ok := s.probe.(bots.EnvSeat)
	return ok && es.WantsEnv(d)
}

func (s *shadowTestSeat) DecideEnv(ctx context.Context, env bots.Env, d decision.Decision) (decision.Intent, error) {
	n, seed := s.index()
	s.envs++
	root, reason := searchseat.HonestRoot(env.Search.Setup, env.Search.Feed, s.live(), bots.RootSeed(seed, d.Seq))
	if root == nil {
		s.refused++
	}
	env.Search.Engine, env.RootRefused = root, reason
	in, err := s.fresh(seed).(bots.EnvSeat).DecideEnv(ctx, env, d)
	return s.finish(d, n, in, err)
}

// shadowBoardSeat adds the BoardSeat face. Only a policy whose own seat is a
// BoardSeat gets it, so the host dispatches the test seat exactly as it
// would dispatch the policy (a type with a DecideBoard method is a
// BoardSeat whether or not the wrapped policy is).
type shadowBoardSeat struct{ *shadowTestSeat }

func (s shadowBoardSeat) DecideBoard(ctx context.Context, b botpolicy.Board, d decision.Decision) (decision.Intent, error) {
	n, seed := s.index()
	in, err := s.fresh(seed).(heuristic.BoardSeat).DecideBoard(ctx, b, d)
	return s.finish(d, n, in, err)
}

func (s *shadowTestSeat) Decide(ctx context.Context, v view.View, d decision.Decision) (decision.Intent, error) {
	n, seed := s.index()
	in, err := s.fresh(seed).Decide(ctx, v, d)
	return s.finish(d, n, in, err)
}

// liveEngineOf returns table id's current match engine (nil before it starts).
func liveEngineOf(r *Registry, id TableID) *rules.Engine {
	r.mu.RLock()
	tb := r.tables[id]
	r.mu.RUnlock()
	if tb == nil {
		return nil
	}
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	if tb.cur == nil {
		return nil
	}
	return tb.cur.e
}

// shadowSeatTable plays a 2-seat bot table whose seat 0 is a shadowTestSeat
// and whose seat 1 is the hosted policy, to completion, and returns the
// registry and the seat.
func shadowSeatTable(t *testing.T, policy string, seed uint64, maxIntents int, mk func(*shadowTestSeat)) (*Registry, *shadowTestSeat) {
	t.Helper()
	o := testOptions(t)
	o.MaxIntents = maxIntents // 0 = play the whole game
	o.BotDeps = sampleBotDeps(t)
	o.BotUnboundedDecisions = true
	var r *Registry
	var ss *shadowTestSeat
	o.Seats = func(names []string, mseed uint64) []seat.Seat {
		ss = newShadowTestSeat(t, policy, o.BotDeps, mseed, func() *rules.Engine { return liveEngineOf(r, "t1") })
		if mk != nil {
			mk(ss)
		}
		opp, err := bots.New(bots.Default, bots.Options{Seed: mseed ^ 2, Deps: o.BotDeps})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := ss.probe.(heuristic.BoardSeat); ok {
			return []seat.Seat{shadowBoardSeat{ss}, opp}
		}
		return []seat.Seat{ss, opp}
	}
	r, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	cfg := TableConfig{ID: "t1", Name: "shadow", Seats: 2, Decks: []string{"a", "b"},
		Seed: seed, Pace: 0, Spectator: view.Omniscient, BotPolicy: bots.Default}
	if err := r.AddTable(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.autoPayManaEnabled() {
		t.Fatal("fixture assumes a manual-pay table: the test seat builds its shadow without AutoPayMana")
	}
	if err := r.Start("t1"); err != nil {
		t.Fatal(err)
	}
	r.Wait("t1")
	ms, err := r.Matches("t1")
	// A capped match ends "crashed" with the intent-cap reason by design
	// (play's MaxIntents guard); its log is a faithful prefix to scan.
	if err != nil || len(ms) != 1 || (maxIntents == 0 && ms[0].State != protocol.MatchFinished) || (maxIntents > 0 && ms[0].State != protocol.MatchFinished && ms[0].State != protocol.MatchCrashed) {
		reason := ""
		if m := ledgerMatch(t, r, "t1"); m != nil {
			m.mu.RLock()
			reason = m.reason
			m.mu.RUnlock()
		}
		t.Fatalf("match did not finish: %+v %v reason: %s", ms, err, reason)
	}
	return r, ss
}

// TestShadowParityZeroDivergences is the key test: a seat that answers every
// decision with a fresh bots.New seeded ShadowSeed(seed, 0, n) through the
// host's own dispatch is, to the scan, indistinguishable from its shadow:
// ShadowScan reports 0 divergences over every decision the seat answered.
// Run for the Board path ("bot") and an EnvSeat policy (sb-tactical).
func TestShadowParityZeroDivergences(t *testing.T) {
	t.Parallel()
	// The search policy is capped: a searched decision costs a whole search,
	// and it is the row whose honest root actually moves the answer.
	for _, row := range []struct {
		policy     string
		maxIntents int
	}{{bots.Default, 0}, {sbtactical.Policy, 0}, {"search", 400}} {
		policy, maxIntents := row.policy, row.maxIntents
		t.Run(policy, func(t *testing.T) {
			t.Parallel()
			r, ss := shadowSeatTable(t, policy, 20261010, maxIntents, nil)
			divs, scanned, err := r.shadowScan("t1", 1, 0, policy)
			if err != nil {
				t.Fatalf("ShadowScan: %v", err)
			}
			// Preconditions: the scan visited exactly the decisions the
			// seat answered, there were enough of them, they were real
			// choices, and (for the Env policy) the Env path really ran.
			if scanned != len(ss.seen) {
				t.Fatalf("scan visited %d decisions, the seat answered %d", scanned, len(ss.seen))
			}
			if scanned < 20 || ss.multi < 5 {
				t.Fatalf("degenerate fixture: %d decisions, %d with a choice", scanned, ss.multi)
			}
			if e, _ := bots.Lookup(policy); e.Env && ss.envs == 0 {
				t.Fatalf("policy %q is an EnvSeat but no decision took the Env path", policy)
			}
			t.Logf("%s: %d decisions, %d with a choice, %d Env, %d roots refused", policy, scanned, ss.multi, ss.envs, ss.refused)
			if len(divs) != 0 {
				t.Fatalf("parity broken: %d divergences, first at intent %d (%s: human %q shadow %q)",
					len(divs), divs[0].Intent, divs[0].Material, divs[0].Human.Label, divs[0].Shadow.Label)
			}
		})
	}
}

// TestShadowScanFindsInjected: a seat that answers like the shadow except at
// two chosen intents is found at exactly those intents, with the material
// reason the predicate gives for the swapped answer.
func TestShadowScanFindsInjected(t *testing.T) {
	t.Parallel()
	type inj struct {
		n    int
		why  string
		alt  []int
		kind decision.Kind
	}
	var injected []inj
	eligible := 0
	r, ss := shadowSeatTable(t, bots.Default, 20261011, 0, func(s *shadowTestSeat) {
		s.deviate = func(d *decision.Decision, n int, b decision.Intent) (decision.Intent, bool) {
			if len(injected) >= 2 {
				return decision.Intent{}, false
			}
			for _, o := range d.Options {
				alt := decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{o.Index}}
				if d.Validate(alt) != nil {
					continue
				}
				if mat, why := divergence.Material(d, alt, b); mat {
					eligible++
					// Skip some eligible decisions so the two injections are
					// spread through the game rather than adjacent.
					if eligible < 4 || (len(injected) == 1 && eligible < 12) {
						return decision.Intent{}, false
					}
					injected = append(injected, inj{n: n, why: why, alt: alt.Choices, kind: d.Kind})
					return alt, true
				}
				break
			}
			return decision.Intent{}, false
		}
	})
	if len(injected) != 2 {
		t.Fatalf("fixture injected %d deviations, want 2 (eligible %d)", len(injected), eligible)
	}
	divs, scanned, err := r.shadowScan("t1", 1, 0, bots.Default)
	if err != nil {
		t.Fatalf("ShadowScan: %v", err)
	}
	if scanned < 20 {
		t.Fatalf("scan visited only %d decisions", scanned)
	}
	if len(divs) != len(injected) {
		t.Fatalf("scan found %d divergences, injected %d (%+v)", len(divs), len(injected), injected)
	}
	for i, in := range injected {
		dv := divs[i]
		if dv.Intent != in.n || dv.Material != in.why || dv.Kind != in.kind {
			t.Fatalf("divergence %d = intent %d %s %s, want intent %d %s %s", i, dv.Intent, dv.Material, dv.Kind, in.n, in.why, in.kind)
		}
		if !slices.Equal(dv.Human.Choices, in.alt) {
			t.Fatalf("divergence %d human choices %v, want %v", i, dv.Human.Choices, in.alt)
		}
		if slices.Equal(dv.Human.Choices, dv.Shadow.Choices) {
			t.Fatalf("divergence %d human and shadow answers are the same: %v", i, dv.Human.Choices)
		}
		if dv.Policy != bots.Default || dv.Table != "t1" || dv.Match != 1 || dv.Seat != 0 ||
			dv.ShadowSeed != divergence.ShadowSeed(ss.matchSeed, 0, in.n) || dv.ID == "" || len(dv.Options) == 0 {
			t.Fatalf("divergence %d record fields wrong: %+v", i, dv)
		}
	}
}

// TestShadowScanSkipsLedgerMarked: the human-seated table's caretaker and
// auto-flagged intents are absent from the scan, and are PRESENT when the
// same match is scanned with an empty ledger — so the absence is the
// ledger's doing, not a fixture that never diverged there.
func TestShadowScanSkipsLedgerMarked(t *testing.T) {
	t.Parallel()
	o := testOptions(t)
	r := undoTable(t, o, "t1")
	submits, autos := driveLedgerMatch(t, r, "t1", 2)
	if submits == 0 || autos == 0 {
		t.Fatalf("fixture degenerate: %d submits, %d auto", submits, autos)
	}
	prov := ledgerProvenance(t, r, "t1")
	marked := map[int]bool{}
	for i, b := range prov {
		if b&(provCaretaker|provAuto) != 0 {
			marked[i] = true
		}
	}
	if len(marked) == 0 {
		t.Fatal("no ledger entry is marked")
	}
	divs, scanned, err := r.shadowScan("t1", 1, 0, bots.Default)
	if err != nil {
		t.Fatalf("ShadowScan: %v", err)
	}
	if len(divs) == 0 || scanned == 0 {
		t.Fatalf("unmarked scan found %d divergences over %d decisions; nothing to compare", len(divs), scanned)
	}
	for _, dv := range divs {
		if marked[dv.Intent] {
			t.Fatalf("divergence at intent %d, which the ledger marks %02x", dv.Intent, prov[dv.Intent])
		}
	}
	// The same match with the marks cleared must diverge at a marked intent.
	m := ledgerMatch(t, r, "t1")
	m.mu.Lock()
	for i := range m.ledger {
		m.ledger[i] = 0
	}
	m.mu.Unlock()
	all, scannedAll, err := r.shadowScan("t1", 1, 0, bots.Default)
	if err != nil {
		t.Fatalf("ShadowScan (cleared): %v", err)
	}
	if scannedAll <= scanned {
		t.Fatalf("clearing the ledger did not widen the scan: %d -> %d decisions", scanned, scannedAll)
	}
	hit := false
	for _, dv := range all {
		if marked[dv.Intent] {
			hit = true
		}
	}
	if !hit || len(all) <= len(divs) {
		t.Fatalf("cleared-ledger scan has %d divergences (%d before), none at a marked intent: the marks were not what excluded them", len(all), len(divs))
	}
}

// TestSnapshotMatchNamesMatchK: on a perpetual table, once match 2 has
// started, SnapshotMatch(id, 1) is match 1's — its head is the finished
// sidecar's — while SnapshotForFeedback follows the current match.
func TestSnapshotMatchNamesMatchK(t *testing.T) {
	t.Parallel()
	o := testOptions(t)
	r, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	cfg := TableConfig{ID: "t1", Name: "perp", Seats: 2, Decks: []string{"a", "b"},
		Seed: 77, Pace: 0, Spectator: view.Omniscient, Perpetual: true}
	if err := r.AddTable(cfg); err != nil {
		t.Fatal(err)
	}
	if err := r.Start("t1"); err != nil {
		t.Fatal(err)
	}
	var first protocol.MatchInfo
	deadline := time.Now().Add(60 * time.Second)
	for {
		ms, err := r.Matches("t1")
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) >= 3 && ms[0].State == protocol.MatchFinished {
			first = ms[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("table never reached a third match: %+v", ms)
		}
		time.Sleep(time.Millisecond)
	}
	if first.Match != 1 || first.Head == "" {
		t.Fatalf("fixture: first match info %+v", first)
	}
	snap, err := r.SnapshotMatch("t1", 1, nil)
	if err != nil {
		t.Fatalf("SnapshotMatch: %v", err)
	}
	if snap.Match.Match != 1 || snap.Log.Head != first.Head || snap.Match.Head != first.Head {
		t.Fatalf("snapshot is match %d head %s / %s, want match 1 head %s", snap.Match.Match, snap.Log.Head, snap.Match.Head, first.Head)
	}
	if snap.View != nil {
		t.Fatal("a nil seat must skip the view")
	}
	cur, err := r.SnapshotForFeedback("t1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Match.Match == 1 {
		t.Fatalf("SnapshotForFeedback still names match 1; the fixture cannot tell the entry points apart")
	}
	if _, err := r.SnapshotMatch("t1", 99, nil); err == nil {
		t.Fatal("SnapshotMatch of an unknown match must fail")
	}
}
