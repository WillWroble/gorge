package eventhost

import (
	"github.com/adams-shaun/gorge/event"
	"github.com/adams-shaun/gorge/host"
	"github.com/adams-shaun/gorge/protocol"
)

// gameRef is the event game attempt one host table is playing for its run.
type gameRef struct {
	run     *run
	round   int
	a, b    int
	game    int
	attempt int
}

// outcome is one finished game match waiting for its pairing-order place
// in the event log: flush applies outcomes only in pairing order, so a
// later series' result waits until every earlier series of the round has
// ended, and two replays log identical entry orders however the games
// finished.
type outcome struct {
	ref gameRef
	m   protocol.MatchInfo
}

// OnMatchEnd is the Host's host.OnMatchEndFunc: every event game table's
// match terminal state arrives here. It ignores non-event tables (k != 1),
// aborted and still-live matches, and tables it does not know. It never
// calls back into the Registry and never takes a run mutex on the calling
// goroutine — the match goroutine that fires it holds nothing this run
// may wait on — so the handling moves onto the Host's wait group.
func (h *Host) OnMatchEnd(t host.TableID, k int, m protocol.MatchInfo) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	if k != 1 || m.State == protocol.MatchAborted || m.State == protocol.MatchLive {
		return nil
	}
	ref, ok := h.tables[t]
	if !ok {
		return nil
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		r := ref.run
		r.mu.Lock()
		r.gameOver(ref, m)
		r.mu.Unlock()
	}()
	return nil
}

// gameOver records one finished match as its series' buffered outcome and
// drives the run forward. The caller holds r.mu. An outcome whose attempt
// is no longer the series' in-flight game — a duplicate or stale callback
// for a table the log has already moved past — is dropped.
func (r *run) gameOver(ref gameRef, m protocol.MatchInfo) {
	live := false
	for _, att := range r.ev.InFlight() {
		if att.Table == m.Table {
			live = true
			break
		}
	}
	if !live {
		return
	}
	r.outcomes[[2]int{ref.a, ref.b}] = outcome{ref: ref, m: m}
	r.h.mu.Lock()
	delete(r.h.tables, host.TableID(m.Table))
	r.h.mu.Unlock()
	r.advance()
	r.broadcast()
}

// flush applies every buffered result that can be logged now, in pairing
// order: only the first pending series, and only once its outcome is
// buffered. The caller holds r.mu.
func (r *run) flush() bool {
	applied := false
	for {
		ps := r.ev.PendingSeries()
		if len(ps) == 0 {
			return applied
		}
		sp := ps[0]
		key := [2]int{sp.A, sp.B}
		o, ok := r.outcomes[key]
		if !ok {
			return applied
		}
		delete(r.outcomes, key)
		if err := r.applyOutcome(sp, o); err != nil {
			return applied
		}
		applied = true
	}
}

// applyOutcome logs one buffered result. A finished match logs GameEnded
// with its winner and head, then closes the series; a crashed match aborts
// the attempt, and once no attempts are left closes the series as a draw.
// The caller holds r.mu.
func (r *run) applyOutcome(sp event.SeriesSpec, o outcome) error {
	a, b := sp.A, sp.B
	switch o.m.State {
	case protocol.MatchFinished:
		var winner *int
		score := [2]int{0, 0}
		if o.m.Winner != nil {
			w := a
			if *o.m.Winner == 1 {
				w = b
			}
			winner = &w
			if w == a {
				score[0] = 1
			} else {
				score[1] = 1
			}
		}
		if err := r.apply(event.Entry{
			Kind: event.KindGameEnded, Round: sp.Round, A: a, B: b,
			Game: o.ref.game, Attempt: o.ref.attempt, Winner: winner, Head: o.m.Head,
		}); err != nil {
			return err
		}
		return r.apply(event.Entry{
			Kind: event.KindSeriesEnded, Round: sp.Round, A: a, B: b, Score: score,
		})
	case protocol.MatchCrashed:
		if err := r.apply(event.Entry{
			Kind: event.KindGameAborted, Round: sp.Round, A: a, B: b,
			Game: o.ref.game, Attempt: o.ref.attempt, Reason: "crashed",
		}); err != nil {
			return err
		}
		if o.ref.attempt < r.maxAttempts() {
			return nil
		}
		head := o.m.Head
		if head == "" {
			head = "crashed"
		}
		if err := r.apply(event.Entry{
			Kind: event.KindGameEnded, Round: sp.Round, A: a, B: b,
			Game: o.ref.game, Attempt: o.ref.attempt, Head: head,
		}); err != nil {
			return err
		}
		return r.apply(event.Entry{
			Kind: event.KindSeriesEnded, Round: sp.Round, A: a, B: b, Score: [2]int{0, 0},
		})
	}
	return nil
}

// maxAttempts is the series-level cap on game attempts: 0 means 3.
func (r *run) maxAttempts() int {
	if r.h.opts.MaxAttempts > 0 {
		return r.h.opts.MaxAttempts
	}
	return 3
}
