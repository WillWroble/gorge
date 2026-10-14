package eventhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	_ "github.com/adams-shaun/gorge/bots/all"
	"github.com/adams-shaun/gorge/event"
	"github.com/adams-shaun/gorge/host"
)

// bo3Series is one pairing's Bo3 entries in log order, grouped by kind.
type bo3Series struct {
	started  event.Entry
	starts   []event.Entry // GameStarted, log order
	aborts   []event.Entry // GameAborted, log order
	ends     []event.Entry // GameEnded, log order
	sideb    []event.Entry // Sideboarded, log order
	finished event.Entry   // SeriesEnded
}

// groupSeries splits a log's series entries per round and pairing,
// preserving order (the same two seats can meet again in a later round).
func groupSeries(log []event.Entry) map[[3]int]*bo3Series {
	out := map[[3]int]*bo3Series{}
	for _, en := range log {
		var key [3]int
		switch en.Kind {
		case event.KindSeriesStarted:
			key = [3]int{en.Round, en.A, en.B}
		case event.KindGameStarted, event.KindGameAborted, event.KindGameEnded, event.KindSideboarded, event.KindSeriesEnded:
			key = [3]int{en.Round, en.A, en.B}
		default:
			continue
		}
		s := out[key]
		if s == nil {
			s = &bo3Series{}
			out[key] = s
		}
		switch en.Kind {
		case event.KindSeriesStarted:
			s.started = en
		case event.KindGameStarted:
			s.starts = append(s.starts, en)
		case event.KindGameAborted:
			s.aborts = append(s.aborts, en)
		case event.KindGameEnded:
			s.ends = append(s.ends, en)
		case event.KindSideboarded:
			s.sideb = append(s.sideb, en)
		case event.KindSeriesEnded:
			s.finished = en
		}
	}
	return out
}

// checkChooserPlan asserts one GameStarted's chooser against the series
// rules: game 1 the table's toss winner (the log's own GameSeed), later
// games MTR 2.2's chooser over the previous ENDED game (its winner, or its
// chooser after a draw).
func checkChooserPlan(t *testing.T, pair [3]int, en event.Entry, prevStart, prevEnd *event.Entry) {
	t.Helper()
	if en.Chooser == nil {
		t.Fatalf("series %v game %d attempt %d has no chooser", pair, en.Game, en.Attempt)
	}
	c := *en.Chooser
	if c != en.A && c != en.B {
		t.Fatalf("series %v game %d chooser %d is not a series seat", pair, en.Game, c)
	}
	if en.Game == 1 {
		toss := host.MatchTossWinner(en.GameSeed, 1, 2)
		want := en.A
		if toss == 1 {
			want = en.B
		}
		if c != want {
			t.Fatalf("series %v game 1 chooser %d, want toss winner %d", pair, c, want)
		}
		return
	}
	want := prevStart.Chooser
	if prevEnd.Winner != nil {
		w := en.A
		if *prevEnd.Winner == en.A {
			w = en.B
		}
		want = &w
	}
	if want == nil || c != *want {
		t.Fatalf("series %v game %d chooser %d, want %v (MTR 2.2)", pair, en.Game, c, want)
	}
}

// checkBo3Shape asserts one played-out (no crash) Bo3 series' log shape and
// returns its SeriesEnded score.
func checkBo3Shape(t *testing.T, pair [3]int, s *bo3Series) [2]int {
	t.Helper()
	if s.started.Kind != event.KindSeriesStarted || s.started.BestOf != 3 {
		t.Fatalf("series %v SeriesStarted = %+v", pair, s.started)
	}
	n := len(s.ends)
	if n != 2 && n != 3 {
		t.Fatalf("series %v played %d games, want 2 or 3", pair, n)
	}
	if len(s.starts) != n || len(s.aborts) != 0 {
		t.Fatalf("series %v starts=%d aborts=%d ends=%d", pair, len(s.starts), len(s.aborts), n)
	}
	wins := [2]int{}
	for i, en := range s.ends {
		if en.Game != i+1 {
			t.Fatalf("series %v game %d ended out of order: %+v", pair, i+1, en)
		}
		if en.Winner != nil {
			if *en.Winner == pair[1] {
				wins[0]++
			} else {
				wins[1]++
			}
		}
	}
	if wins[0] < 2 && wins[1] < 2 && n != 3 {
		t.Fatalf("series %v undecided after %d games: %v", pair, n, wins)
	}
	table := s.starts[0].Table
	for i, en := range s.starts {
		if en.Game != i+1 || en.Attempt != 1 {
			t.Fatalf("series %v start %d = game %d attempt %d", pair, i, en.Game, en.Attempt)
		}
		if en.Table != table {
			t.Fatalf("series %v game %d moved table %s -> %s without a crash", pair, en.Game, table, en.Table)
		}
		var prevStart, prevEnd *event.Entry
		if i > 0 {
			prevStart, prevEnd = &s.starts[i-1], &s.ends[i-1]
		}
		checkChooserPlan(t, pair, en, prevStart, prevEnd)
	}
	for g := 2; g <= n; g++ {
		seen := map[int]bool{}
		for _, en := range s.sideb {
			if en.Game != g {
				continue
			}
			if en.Rev != 1 || seen[en.Seat] || (en.Seat != pair[1] && en.Seat != pair[2]) {
				t.Fatalf("series %v game %d sideboard %+v", pair, g, en)
			}
			seen[en.Seat] = true
		}
		if len(seen) != 2 {
			t.Fatalf("series %v game %d sideboarded by %d sides", pair, g, len(seen))
		}
	}
	if s.finished.Kind != event.KindSeriesEnded || s.finished.Score != wins {
		t.Fatalf("series %v SeriesEnded = %+v, wins %v", pair, s.finished, wins)
	}
	return wins
}

// TestBotOnlyBo3 drives a bot-only best-of-three event to COMPLETE and
// checks every series' log: SeriesStarted 3, one series table per series
// (games 2/3 continue on it, chooser per MTR 2.2), bots sideboarding
// between games, SeriesEnded with the games' score.
func TestBotOnlyBo3(t *testing.T) {
	p := startGamePair(t, "", nil)
	req := botEventReq("bot", "lethal-pressure", "cast-profile", "bot")
	req.BestOf = 3
	resp := createOK(t, p.h, req)
	waitPhase(t, p.h, resp.Event, "COMPLETE", 50*time.Second)

	r := runOf(t, p.h, resp.Event)
	r.mu.Lock()
	log, st := r.ev.Log(), r.stateFor(-1)
	r.mu.Unlock()

	if st.BestOf != 3 {
		t.Fatalf("state best_of = %d, want 3", st.BestOf)
	}
	if len(log) == 0 || log[len(log)-1].Kind != event.KindEventEnded {
		t.Fatalf("event did not end: last entry %+v", log[len(log)-1])
	}
	perSeries := groupSeries(log)
	if len(perSeries) != 6 {
		t.Fatalf("%d series logged, want 6", len(perSeries))
	}
	points := [4]int{}
	for pair, s := range perSeries {
		wins := checkBo3Shape(t, pair, s)
		switch {
		case wins[0] == 2:
			points[pair[1]] += 3
		case wins[1] == 2:
			points[pair[2]] += 3
		default:
			points[pair[1]], points[pair[2]] = points[pair[1]]+1, points[pair[2]]+1
		}
	}
	r.mu.Lock()
	standings := r.ev.Standings()
	r.mu.Unlock()
	if len(standings) != 4 {
		t.Fatalf("%d standings rows, want 4", len(standings))
	}
	for _, row := range standings {
		if row.Points != points[row.Seat] {
			t.Fatalf("seat %d points %d, want %d", row.Seat, row.Points, points[row.Seat])
		}
	}
}

// TestBo1EventUnchanged pins the Bo1 shape against the Bo3 additions: the
// event state carries best_of 1 and every series row keeps exactly the E1
// fields (no game, games, chooser or waiting), and the log has no
// Sideboarded, no chooser and no best-of-three SeriesStarted.
func TestBo1EventUnchanged(t *testing.T) {
	p := startGamePair(t, "", nil)
	r := playBotEvent(t, p)

	r.mu.Lock()
	log, st := r.ev.Log(), r.stateFor(-1)
	r.mu.Unlock()

	if st.BestOf != 1 {
		t.Fatalf("state best_of = %d, want 1", st.BestOf)
	}
	for i, ser := range st.Series {
		if ser.BestOf != 1 || ser.Game != 0 || ser.Games != nil || ser.Chooser != nil || ser.Waiting != nil {
			t.Fatalf("series row %d grew Bo3 fields: %+v", i, ser)
		}
	}
	b, err := json.Marshal(&st)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"chooser"`, `"waiting"`, `"games"`} {
		if bytes.Contains(b, []byte(key)) {
			t.Fatalf("Bo1 event state JSON contains %s: %s", key, b)
		}
	}
	for _, en := range log {
		switch en.Kind {
		case event.KindSeriesStarted:
			if en.BestOf != 1 {
				t.Fatalf("SeriesStarted best_of %d, want 1", en.BestOf)
			}
		case event.KindGameStarted:
			if en.Chooser != nil {
				t.Fatalf("Bo1 GameStarted carries a chooser: %+v", en)
			}
		case event.KindSideboarded:
			t.Fatalf("Bo1 log has a Sideboarded: %+v", en)
		}
	}
}

// TestBo3CrashRetriesOnNewTable stalls every game (MaxDecisionsPerTurn 1)
// and caps attempts at 2, so every Bo3 game crashes twice and logs as a
// draw: each attempt restarts on a NEW table, the series plays its full
// three games and ends 0-0, and every game's chooser follows the rules —
// game 1 per attempt the attempt's own toss winner, later games the
// previous drawn game's chooser.
func TestBo3CrashRetriesOnNewTable(t *testing.T) {
	p := startGamePair(t, "", func(eo *Options, ho *host.Options) {
		eo.MaxAttempts = 2
		ho.MaxDecisionsPerTurn = 1
	})
	req := botEventReq("bot", "lethal-pressure", "cast-profile", "bot")
	req.BestOf = 3
	resp := createOK(t, p.h, req)
	waitPhase(t, p.h, resp.Event, "COMPLETE", 50*time.Second)

	r := runOf(t, p.h, resp.Event)
	r.mu.Lock()
	log := r.ev.Log()
	r.mu.Unlock()

	perSeries := groupSeries(log)
	if len(perSeries) != 6 {
		t.Fatalf("%d series logged, want 6", len(perSeries))
	}
	for pair, s := range perSeries {
		if s.started.Kind != event.KindSeriesStarted || s.started.BestOf != 3 {
			t.Fatalf("series %v SeriesStarted = %+v", pair, s.started)
		}
		if len(s.ends) != 3 || len(s.starts) != 6 || len(s.aborts) != 6 {
			t.Fatalf("series %v starts=%d aborts=%d ends=%d, want 6/6/3",
				pair, len(s.starts), len(s.aborts), len(s.ends))
		}
		// Every game is drawn, so games 2 and 3 re-use game 1's chooser
		// (MTR 2.2: a drawn game's chooser chooses again).
		var starts1 []event.Entry
		var end1 event.Entry
		for _, en := range s.starts {
			if en.Game == 1 {
				starts1 = append(starts1, en)
			}
		}
		for _, en := range s.ends {
			if en.Game == 1 {
				end1 = en
			}
		}
		if len(starts1) != 2 || end1.Kind != event.KindGameEnded {
			t.Fatalf("series %v game 1 starts=%d end=%+v", pair, len(starts1), end1)
		}
		tables := map[string]bool{}
		// Log order per game g: start(a1), abort(a1), start(a2), abort(a2), end.
		for g := 1; g <= 3; g++ {
			var gStarts, gAborts []event.Entry
			for _, en := range s.starts {
				if en.Game == g {
					gStarts = append(gStarts, en)
				}
			}
			for _, en := range s.aborts {
				if en.Game == g {
					gAborts = append(gAborts, en)
				}
			}
			if len(gStarts) != 2 || len(gAborts) != 2 {
				t.Fatalf("series %v game %d starts=%d aborts=%d", pair, g, len(gStarts), len(gAborts))
			}
			for i, en := range gStarts {
				if en.Attempt != i+1 {
					t.Fatalf("series %v game %d start %d has attempt %d", pair, g, i, en.Attempt)
				}
				if tables[en.Table] {
					t.Fatalf("series %v game %d attempt %d reused table %s", pair, g, en.Attempt, en.Table)
				}
				tables[en.Table] = true
				var prevStart, prevEnd *event.Entry
				if g > 1 {
					prevStart, prevEnd = &starts1[len(starts1)-1], &end1
				}
				checkChooserPlan(t, pair, en, prevStart, prevEnd)
			}
			for _, en := range gAborts {
				if en.Reason != "crashed" || en.Attempt > 2 {
					t.Fatalf("series %v game %d abort %+v", pair, g, en)
				}
			}
		}
		for _, en := range s.ends {
			if en.Winner != nil || en.Head == "" {
				t.Fatalf("series %v GameEnded = %+v, want a headless draw with a head", pair, en)
			}
		}
		if s.finished.Kind != event.KindSeriesEnded || s.finished.Score != [2]int{0, 0} {
			t.Fatalf("series %v SeriesEnded = %+v", pair, s.finished)
		}
	}
}

// seriesRes is one Host.Series call's result, for parked-call tests.
type seriesRes struct {
	g  host.SeriesGame
	ok bool
}

// parkSeries calls h.Series on a goroutine and returns the result channel.
func parkSeries(h *Host, ctx context.Context, id host.TableID, k int) <-chan seriesRes {
	ch := make(chan seriesRes, 1)
	go func() {
		g, ok, _ := h.Series(ctx, id, k)
		ch <- seriesRes{g: g, ok: ok}
	}()
	return ch
}

// assertParked fails if the parked call answers within d.
func assertParked(t *testing.T, ch <-chan seriesRes, d time.Duration) {
	t.Helper()
	select {
	case res := <-ch:
		t.Fatalf("parked series call answered early: %+v (ok %v)", res.g, res.ok)
	case <-time.After(d):
	}
}

// assertAnswered fails unless the parked call answers within d.
func assertAnswered(t *testing.T, ch <-chan seriesRes, d time.Duration) seriesRes {
	t.Helper()
	select {
	case res := <-ch:
		return res
	case <-time.After(d):
		t.Fatalf("parked series call never answered within %s", d)
		return seriesRes{}
	}
}

// TestSeriesHookWaitsAndCloseReleases drives the Host.Series hook against
// a live Bo3 series table whose human seat never acts, so game 1 stays in
// flight: match 1 plans ready with the two saved decks and no chooser (the
// seeded toss decides), match 2 parks on the run until the log moves or
// the call is released, an unknown or non-series table answers no, a
// cancelled context returns promptly, and Host.Close releases the parked
// call and returns.
func TestSeriesHookWaitsAndCloseReleases(t *testing.T) {
	dir := t.TempDir()
	mut := func(eo *Options, _ *host.Options) {
		eo.MintSeat = func(table host.TableID, seat int) (string, error) {
			return fmt.Sprintf("tok-%s-%d", table, seat), nil
		}
	}
	p := startGamePair(t, dir, mut)
	req := oneHumanReq()
	req.BestOf = 3
	resp := createOK(t, p.h, req)
	id := resp.Event
	tok, _ := humanToken(t, resp)
	joinOK(t, p.h, id, tok, "Ann")
	pickTop(t, p.h, id, tok, 45)
	finishHumanBuild(t, p.h, id, tok)
	waitPhase(t, p.h, id, "ROUND1", 50*time.Second)

	r := runOf(t, p.h, id)
	r.mu.Lock()
	tableID, seriesOK := r.seriesTable[[2]int{0, 3}]
	r.mu.Unlock()
	if !seriesOK {
		t.Fatal("seat 0's series has no table")
	}

	// Match 1's plan is ready: both saved decks, chooser -1 (the seeded
	// toss decides).
	res := assertAnswered(t, parkSeries(p.h, context.Background(), tableID, 1), 10*time.Second)
	if !res.ok || res.g.Chooser != -1 || len(res.g.Decks) != 2 ||
		res.g.Decks[0] != DeckID(id, 0, 1) || res.g.Decks[1] != DeckID(id, 3, 1) {
		t.Fatalf("match 1 plan = %+v ok=%v", res.g, res.ok)
	}

	// An unknown table and a non-series table answer no.
	if g, ok, err := p.h.Series(context.Background(), "no-such-table", 1); ok || err != nil || g.Chooser != 0 {
		t.Fatalf("unknown table plan = %+v ok=%v err=%v", g, ok, err)
	}
	botResp := createOK(t, p.h, botEventReq("bot", "lethal-pressure", "cast-profile", "bot"))
	waitPhase(t, p.h, botResp.Event, "ROUND1", 50*time.Second)
	rb1 := runOf(t, p.h, botResp.Event)
	rb1.mu.Lock()
	bo1Tables := map[host.TableID]bool{}
	for _, att := range rb1.ev.InFlight() {
		bo1Tables[host.TableID(att.Table)] = true
	}
	rb1.mu.Unlock()
	for tid := range bo1Tables {
		if g, ok, err := p.h.Series(context.Background(), tid, 1); ok || err != nil {
			t.Fatalf("Bo1 table %s plan = %+v ok=%v err=%v", tid, g, ok, err)
		}
	}

	// Match 2 parks while game 1 is in flight, and a cancelled context
	// unblocks it.
	ctx, cancel := context.WithCancel(context.Background())
	ch := parkSeries(p.h, ctx, tableID, 2)
	assertParked(t, ch, 500*time.Millisecond)
	cancel()
	if res := assertAnswered(t, ch, 10*time.Second); res.ok {
		t.Fatalf("cancelled park answered ok: %+v", res.g)
	}

	// A second park is released by Close: the call answers no and Close
	// returns.
	ch = parkSeries(p.h, context.Background(), tableID, 2)
	assertParked(t, ch, 500*time.Millisecond)
	closed := make(chan error, 1)
	go func() { closed <- p.h.Close() }()
	if res := assertAnswered(t, ch, 10*time.Second); res.ok {
		t.Fatalf("Close-released park answered ok: %+v", res.g)
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return with a parked series call")
	}
}

// TestResumeBetweenGamesContinuesOnANewTable restarts a bot-only Bo3 event
// mid-round from its store: the in-flight attempts are aborted, the series
// resume on fresh tables (a between-games series continues with its locked
// decks, a mid-game one replays on a new one), and the event plays out to
// COMPLETE with every series' log shape intact.
func TestResumeBetweenGamesContinuesOnANewTable(t *testing.T) {
	dir := t.TempDir()
	p := startGamePair(t, dir, nil)
	req := botEventReq("bot", "lethal-pressure", "cast-profile", "bot")
	req.BestOf = 3
	resp := createOK(t, p.h, req)
	id := resp.Event
	waitPhase(t, p.h, id, "ROUND1", 50*time.Second)
	r := runOf(t, p.h, id)
	r.mu.Lock()
	log1 := r.ev.Log()
	r.mu.Unlock()
	oldTables := map[string]bool{}
	for _, en := range log1 {
		if en.Kind == event.KindGameStarted {
			oldTables[en.Table] = true
		}
	}
	if len(oldTables) == 0 {
		t.Fatal("no games started before the restart")
	}

	p.h.Close()
	p.reg.Close()

	p2 := startGamePair(t, dir, nil)
	waitPhase(t, p2.h, id, "COMPLETE", 50*time.Second)
	r2 := runOf(t, p2.h, id)
	r2.mu.Lock()
	log2 := r2.ev.Log()
	r2.mu.Unlock()
	if log2[len(log2)-1].Kind != event.KindEventEnded {
		t.Fatalf("resumed event did not end: last entry %+v", log2[len(log2)-1])
	}
	// The store's log is append-only: the resumed replay is the old log
	// plus what the resumed run added.
	if len(log2) < len(log1) {
		t.Fatalf("resumed log shrank: %d < %d entries", len(log2), len(log1))
	}
	for i, en := range log1 {
		if !reflect.DeepEqual(en, log2[i]) {
			t.Fatalf("resumed log diverges at %d: %+v vs %+v", i, en, log2[i])
		}
	}
	// Every GameStarted the resumed run logged is on a table the first
	// host never used: an aborted attempt replays on a fresh table, and a
	// between-games series continues on a fresh one (the first host's
	// series tables died with it).
	for _, en := range log2[len(log1):] {
		if en.Kind == event.KindGameStarted && oldTables[en.Table] {
			t.Fatalf("resumed run reused first host's table %s: %+v", en.Table, en)
		}
	}
	perSeries := groupSeries(log2)
	if len(perSeries) != 6 {
		t.Fatalf("%d series logged, want 6", len(perSeries))
	}
	for pair, s := range perSeries {
		if s.started.Kind != event.KindSeriesStarted || s.started.BestOf != 3 {
			t.Fatalf("series %v SeriesStarted = %+v", pair, s.started)
		}
		if len(s.ends) < 2 || len(s.ends) > 3 {
			t.Fatalf("series %v played %d games", pair, len(s.ends))
		}
		wins := [2]int{}
		for _, en := range s.ends {
			if en.Winner != nil {
				if *en.Winner == pair[1] {
					wins[0]++
				} else {
					wins[1]++
				}
			}
		}
		if wins[0] < 2 && wins[1] < 2 && len(s.ends) != 3 {
			t.Fatalf("series %v undecided after %d games: %v", pair, len(s.ends), wins)
		}
		if s.finished.Kind != event.KindSeriesEnded || s.finished.Score != wins {
			t.Fatalf("series %v SeriesEnded = %+v, wins %v", pair, s.finished, wins)
		}
		// Per game: every attempt's chooser follows the rules across the
		// restart (game 1 the attempt's toss winner, later games MTR 2.2
		// over the previous ended game). A between-games series may keep
		// playing games 2 and 3 on its own table when no restart intervened.
		startsByGame := map[int][]event.Entry{}
		endByGame := map[int]*event.Entry{}
		for _, en := range s.ends {
			e := en
			endByGame[en.Game] = &e
		}
		for _, en := range s.starts {
			startsByGame[en.Game] = append(startsByGame[en.Game], en)
		}
		for g := 1; g <= len(s.ends); g++ {
			sts := startsByGame[g]
			if len(sts) == 0 {
				t.Fatalf("series %v game %d never started", pair, g)
			}
			for i, en := range sts {
				if en.Attempt != i+1 {
					t.Fatalf("series %v game %d start %d has attempt %d", pair, g, i, en.Attempt)
				}
				var prevStart, prevEnd *event.Entry
				if g > 1 {
					prevStart, prevEnd = &startsByGame[g-1][len(startsByGame[g-1])-1], endByGame[g-1]
				}
				checkChooserPlan(t, pair, en, prevStart, prevEnd)
			}
		}
	}
}

// TestBo3CreateValidation pins the create-time best_of grammar: 2 and 4
// are bad requests, 0, 1 and 3 are accepted and reported back in the
// event's state (0 as 1).
func TestBo3CreateValidation(t *testing.T) {
	h, _ := newHost(t, nil)
	req := botEventReq("bot", "bot", "bot", "bot")
	for _, bad := range []int{2, 4, -1} {
		req.BestOf = bad
		_, err := h.CreateEvent(req)
		e, ok := err.(*Error)
		if !ok || e.Status() != 400 {
			t.Fatalf("CreateEvent best_of %d: %v, want a 400", bad, err)
		}
	}
	for _, good := range []int{0, 1, 3} {
		req.BestOf = good
		resp, err := h.CreateEvent(req)
		if err != nil {
			t.Fatalf("CreateEvent best_of %d: %v", good, err)
		}
		st := stateOf(t, h, resp.Event, "")
		want := good
		if want == 0 {
			want = 1
		}
		if st.BestOf != want {
			t.Fatalf("best_of %d: state reports %d, want %d", good, st.BestOf, want)
		}
	}
}
