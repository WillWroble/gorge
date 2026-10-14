package eventhost

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/event"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/protocol"
	"github.com/adams-shaun/gorge/state"
)

// humanSeatsReq builds a four-seat request with the given event seats
// human, the rest bots.
func humanSeatsReq(humans []int, bestOf int) protocol.CreateEventRequest {
	req := protocol.CreateEventRequest{Set: "FDN", BestOf: bestOf}
	for i := 0; i < 4; i++ {
		s := protocol.CreateEventSeat{Kind: "bot"}
		for _, h := range humans {
			if h == i {
				s.Kind = "human"
			}
		}
		req.Seats = append(req.Seats, s)
	}
	return req
}

// inviteToken returns the create response's invite token for seat.
func inviteToken(t *testing.T, resp protocol.CreateEventResponse, seat int) string {
	t.Helper()
	for _, inv := range resp.Invites {
		if inv.Seat == seat {
			return inv.Token
		}
	}
	t.Fatalf("no invite for seat %d", seat)
	return ""
}

// legalIntent answers d with the first Min options: always valid per
// Decision.Validate for any Kind, so the driver needs no rules knowledge
// and can answer whatever decision the engine actually asks the human.
func legalIntent(d *decision.Decision) decision.Intent {
	n := d.Min
	if n > len(d.Options) {
		n = len(d.Options)
	}
	ch := make([]int, 0, n)
	for i := 0; i < n; i++ {
		ch = append(ch, d.Options[i].Index)
	}
	return decision.Intent{Seq: d.Seq, Player: d.Player, Choices: ch}
}

// humanDriveUntilBetween answers every decision the two human seats of
// series (a,b) are asked with the first Min options until the series
// reaches its between window. The driver follows the series' live table
// across crash retries (each retry logs a new table) and fails the test on
// deadline. The human seats sit at table slots 0 and 1 (event seat a at
// slot 0, b at slot 1, match 1).
func humanDriveUntilBetween(t *testing.T, reg *host.Registry, r *run, a, b int) {
	t.Helper()
	deadline := time.Now().Add(50 * time.Second)
	var last [2]uint64
	for {
		r.mu.Lock()
		table := r.seriesTable[[2]int{a, b}]
		v, ok := r.ev.SeriesState(a, b)
		r.mu.Unlock()
		if ok && v.Between {
			return
		}
		if ok && v.Ended {
			t.Fatalf("series (%d,%d) ended before between: %+v", a, b, v)
		}
		if table == "" {
			if time.Now().After(deadline) {
				t.Fatalf("series (%d,%d): no game table within deadline", a, b)
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		acted := false
		for slot := 0; slot < 2; slot++ {
			d, err := reg.Pending(table, 1, state.PlayerID(slot))
			if err != nil || d.Seq == last[slot] {
				continue
			}
			if err := reg.SubmitIntent(table, 1, state.PlayerID(slot), legalIntent(d)); err != nil {
				t.Fatalf("SubmitIntent slot %d: %v", slot, err)
			}
			last[slot] = d.Seq
			acted = true
		}
		if time.Now().After(deadline) {
			t.Fatalf("series (%d,%d) never reached between within deadline", a, b)
		}
		if !acted {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// waitSeriesInFlight polls until series (a,b) has the named game in flight
// and returns its attempt. tableOut, when non-nil, receives the live table.
func waitSeriesInFlight(t *testing.T, r *run, a, b, game int, d time.Duration) (event.GameAttempt, host.TableID) {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		r.mu.Lock()
		v, ok := r.ev.SeriesState(a, b)
		table := r.seriesTable[[2]int{a, b}]
		r.mu.Unlock()
		if ok && v.InFlight != nil && v.InFlight.Game == game {
			return *v.InFlight, table
		}
		if time.Now().After(deadline) {
			if !ok {
				t.Fatalf("series (%d,%d) missing after deadline", a, b)
			}
			t.Fatalf("series (%d,%d) game %d never started: %+v", a, b, game, v)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// pickTwoHumans drives two human seats through the draft, alternating
// picks: a seat's next pack arrives through the other human's pick, so
// neither can be driven to completion alone.
func pickTwoHumans(t *testing.T, h *Host, id, tok0, tok3 string) {
	t.Helper()
	toks := []string{tok0, tok3}
	done := func(tok string) bool {
		return stateOf(t, h, id, tok).Phase == "BUILD"
	}
	for i := 0; i < 400 && !(done(toks[0]) && done(toks[1])); i++ {
		for seat, tok := range toks {
			if done(tok) {
				continue
			}
			p := packOf(t, h, id, tok)
			if p.Waiting {
				continue
			}
			if _, err := h.Pick(id, tok, protocol.PickRequest{
				Round: p.Round, PickNo: p.PickNo, ID: p.Pack[0].ID,
			}); err != nil {
				t.Fatalf("pick seat %d: %v", seat, err)
			}
		}
	}
	if !done(toks[0]) || !done(toks[1]) {
		t.Fatalf("draft stalled: phases %s / %s", stateOf(t, h, id, toks[0]).Phase, stateOf(t, h, id, toks[1]).Phase)
	}
}

func seriesRow(st protocol.EventState, a, b int) *protocol.EventSeries {
	for i := range st.Series {
		if st.Series[i].A == a && st.Series[i].B == b {
			return &st.Series[i]
		}
	}
	return nil
}

func TestMaxHumansDefaultsToFour(t *testing.T) {
	h, _ := newHost(t, nil)
	if got := h.maxHumans(); got != 4 {
		t.Fatalf("maxHumans() = %d, want 4 (E3-5)", got)
	}
	for n := 2; n <= 4; n++ {
		if _, err := h.CreateEvent(humanSeatsReq([]int{0, 1, 2, 3}[:n], 1)); err != nil {
			t.Fatalf("CreateEvent(%d humans): %v", n, err)
		}
	}
}

// TestTwoHumanBo3SeriesSideboardAndKeep is the E3-5 end-to-end: two humans
// (seats 0 and 3) meet in round 1 of a Bo3, play game 1 driven through the
// registry's Pending/SubmitIntent, both sideboard (one Keep, one a new
// deck revision) and the next game starts on the series' own table with
// per-slot join links. The pools stay hidden behind each seat's token.
func TestTwoHumanBo3SeriesSideboardAndKeep(t *testing.T) {
	p := startGamePair(t, "", func(eo *Options, _ *host.Options) {
		eo.MintSeat = func(table host.TableID, seat int) (string, error) {
			return fmt.Sprintf("tok-%s-%d", table, seat), nil
		}
	})
	resp := createOK(t, p.h, humanSeatsReq([]int{0, 3}, 3))
	id := resp.Event
	tok0 := inviteToken(t, resp, 0)
	tok3 := inviteToken(t, resp, 3)
	joinOK(t, p.h, id, tok0, "Ann")
	joinOK(t, p.h, id, tok3, "Bob")
	pickTwoHumans(t, p.h, id, tok0, tok3)
	finishHumanBuild(t, p.h, id, tok0)
	finishHumanBuild(t, p.h, id, tok3)
	waitPhase(t, p.h, id, "ROUND1", 50*time.Second)

	r := runOf(t, p.h, id)
	humanDriveUntilBetween(t, p.reg, r, 0, 3)
	st0 := stateOf(t, p.h, id, tok0)
	row := seriesRow(st0, 0, 3)
	if row == nil || row.State != "between" || row.Game != 2 {
		t.Fatalf("seat 0 series row = %+v, want between/game 2", row)
	}

	// A stale game number, a both-set body and a neither-set body are all
	// refused, in that order.
	_, err := p.h.Sideboard(id, tok0, protocol.SideboardRequest{Game: 3, Keep: true})
	wantErr(t, err, codeConflict, 409)
	_, err = p.h.Sideboard(id, tok0, protocol.SideboardRequest{Game: 2, Keep: true,
		Deck: &protocol.DeckRequest{Main: []string{"x"}}})
	wantErr(t, err, codeBadRequest, 400)
	_, err = p.h.Sideboard(id, tok0, protocol.SideboardRequest{Game: 2})
	wantErr(t, err, codeBadRequest, 400)

	// While between, the pool already names the deck game 1 was played
	// with: revision 1.
	pool0, err := p.h.EventPool(id, tok0)
	if err != nil {
		t.Fatalf("EventPool(seat 0): %v", err)
	}
	if pool0.Playing == nil || pool0.PlayingRev != 1 {
		t.Fatalf("seat 0 pool Playing = %+v/%d, want rev 1", pool0.Playing, pool0.PlayingRev)
	}

	// Seat 0 keeps the deck it played: the lock lands, Waiting drops it.
	if _, err := p.h.Sideboard(id, tok0, protocol.SideboardRequest{Game: 2, Keep: true}); err != nil {
		t.Fatalf("Sideboard(keep): %v", err)
	}
	r.mu.Lock()
	v, ok := r.ev.SeriesState(0, 3)
	r.mu.Unlock()
	if !ok || v.Locked != [2]int{1, 0} {
		t.Fatalf("Locked after seat 0 keep = %+v", v)
	}
	if _, err := p.h.Sideboard(id, tok0, protocol.SideboardRequest{Game: 2, Keep: true}); err == nil {
		t.Fatal("second Sideboard by seat 0 succeeded")
	}

	// Neither pool body carries the other human's cards.
	pool3, err := p.h.EventPool(id, tok3)
	if err != nil {
		t.Fatalf("EventPool(seat 3): %v", err)
	}
	b0, _ := json.Marshal(pool0)
	b3, _ := json.Marshal(pool3)
	for _, c := range pool3.Pool {
		if strings.Contains(string(b0), `"`+c.ID+`"`) {
			t.Errorf("seat 0's pool body leaks seat 3 card %q", c.ID)
		}
	}
	for _, c := range pool0.Pool {
		if strings.Contains(string(b3), `"`+c.ID+`"`) {
			t.Errorf("seat 3's pool body leaks seat 0 card %q", c.ID)
		}
	}

	// Seat 3 sideboards a mutated suggestion: a new revision, locked and
	// served as the playing deck.
	d := pool3.Suggestion
	if len(d.Main) < 2 {
		t.Fatalf("suggestion too small: %+v", d)
	}
	d.Main[0], d.Main[1] = d.Main[1], d.Main[0]
	if _, err := p.h.Sideboard(id, tok3, protocol.SideboardRequest{Game: 2, Deck: &d}); err != nil {
		t.Fatalf("Sideboard(deck): %v", err)
	}
	pool3, err = p.h.EventPool(id, tok3)
	if err != nil {
		t.Fatalf("EventPool(seat 3) after save: %v", err)
	}
	if pool3.Rev != 2 || pool3.PlayingRev != 2 || pool3.Playing == nil {
		t.Fatalf("seat 3 pool after sideboard: rev %d playing %d/%+v, want rev 2", pool3.Rev, pool3.PlayingRev, pool3.Playing)
	}
	got := *pool3.Playing
	if len(got.Main) != len(d.Main) || len(got.Basics) != len(d.Basics) {
		t.Fatalf("playing deck = %+v, want %+v", got, d)
	}
	for i := range d.Main {
		if got.Main[i] != d.Main[i] {
			t.Fatalf("playing main[%d] = %q, want %q", i, got.Main[i], d.Main[i])
		}
	}

	// The next game starts on the series' own table; each seat's join link
	// names its own slot and token.
	att, table := waitSeriesInFlight(t, r, 0, 3, 2, 50*time.Second)
	if att.Game != 2 {
		t.Fatalf("game 2 attempt = %+v", att)
	}
	st0 = stateOf(t, p.h, id, tok0)
	st3 := stateOf(t, p.h, id, tok3)
	row0, row3 := seriesRow(st0, 0, 3), seriesRow(st3, 0, 3)
	want0 := fmt.Sprintf("/t/%s?seat=0&token=tok-%s-0&event=%s", table, table, id)
	want3 := fmt.Sprintf("/t/%s?seat=1&token=tok-%s-1&event=%s", table, table, id)
	if row0 == nil || row0.Join != want0 {
		t.Fatalf("seat 0 join = %+v, want %q", row0, want0)
	}
	if row3 == nil || row3.Join != want3 {
		t.Fatalf("seat 3 join = %+v, want %q", row3, want3)
	}

	// The log carries both Sideboarded entries with the revisions played.
	r.mu.Lock()
	entries := append([]event.Entry(nil), r.ev.Log()...)
	r.mu.Unlock()
	saw := map[int]int{} // seat -> Rev
	for _, en := range entries {
		if en.Kind == event.KindSideboarded && en.A == 0 && en.B == 3 && en.Game == 2 {
			if en.Auto {
				t.Errorf("human Sideboarded entry for seat %d is Auto", en.Seat)
			}
			saw[en.Seat] = en.Rev
		}
	}
	if saw[0] != 1 || saw[3] != 2 {
		t.Errorf("sideboarded revisions = %v, want seat 0 rev 1, seat 3 rev 2", saw)
	}
}

// TestSideboardErrors refuses a sideboard outside a round's between
// window: in BUILD the deck flow is save/ready, not sideboard.
func TestSideboardErrors(t *testing.T) {
	p := startGamePair(t, "", nil)
	resp := createOK(t, p.h, oneHumanReq())
	id := resp.Event
	tok, _ := humanToken(t, resp)
	joinOK(t, p.h, id, tok, "Ann")
	pickTop(t, p.h, id, tok, 1)
	_, err := p.h.Sideboard(id, tok, protocol.SideboardRequest{Game: 1, Keep: true})
	wantErr(t, err, codeConflict, 409)
}

// TestIdleSeatAutoKeepsBetweenGames is the E1-8 half: a human that played
// game 1 and then goes idle keeps the deck it played for game 2 (one
// auto-keep, Auto on the entry) and — still idle once game 2 is live —
// loses the series by concession in the same sweep.
func TestIdleSeatAutoKeepsBetweenGames(t *testing.T) {
	clock := newTestClock()
	p := startGamePair(t, "", func(eo *Options, _ *host.Options) {
		eo.MintSeat = func(table host.TableID, seat int) (string, error) {
			return fmt.Sprintf("tok-%s-%d", table, seat), nil
		}
		eo.IdleTimeout = 30 * time.Minute
		eo.Now = clock.Now
	})
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
	humanDriveUntilBetween(t, p.reg, r, 0, 3)
	r.mu.Lock()
	v, ok := r.ev.SeriesState(0, 3)
	r.mu.Unlock()
	if !ok || !v.Between || len(v.Games) != 1 || v.InFlight != nil {
		t.Fatalf("series before the idle sweep = %+v, want between after game 1", v)
	}
	w1 := v.Games[0]
	rev := v.PlayRevs[0]

	// The seat goes idle; one sweep auto-keeps, starts game 2 and, the
	// seat still owing its game decisions, concedes the series.
	clock.Add(30*time.Minute + time.Minute)
	p.h.checkTimeouts(clock.Now())

	r.mu.Lock()
	entries := append([]event.Entry(nil), r.ev.Log()...)
	phase := r.ev.Phase()
	r.mu.Unlock()
	// The idle path may have finished round 1 entirely (the bot series
	// plays fast too), so the (0,3) pairing may already be re-paired away:
	// the assertions read the log.
	var gotKeep, sawConcedeGame2, sawSeriesEnd bool
	wantScore := [2]int{0, 2}
	if w1 == 0 {
		wantScore = [2]int{1, 2}
	}
	for _, en := range entries {
		switch {
		case en.Kind == event.KindSideboarded && en.A == 0 && en.B == 3 && en.Game == 2:
			gotKeep = true
			if !en.Auto {
				t.Errorf("auto-keep Sideboarded entry is not Auto: %+v", en)
			}
			if en.Seat != 0 || en.Rev != rev {
				t.Errorf("auto-keep Sideboarded = seat %d rev %d, want seat 0 rev %d", en.Seat, en.Rev, rev)
			}
		case en.Kind == event.KindGameEnded && en.A == 0 && en.B == 3 && en.Game == 2 && en.Head == "conceded":
			sawConcedeGame2 = true
		case en.Kind == event.KindSeriesEnded && en.A == 0 && en.B == 3:
			sawSeriesEnd = true
			if en.Score != wantScore {
				t.Errorf("series score = %v, want %v (game 1 winner %d)", en.Score, wantScore, w1)
			}
		}
	}
	if !gotKeep || !sawConcedeGame2 || !sawSeriesEnd {
		t.Fatalf("sweep log (phase %s): keep %v, conceded game 2 %v, series ended %v", phase, gotKeep, sawConcedeGame2, sawSeriesEnd)
	}
}
