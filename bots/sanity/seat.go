package sanity

import (
	"context"

	"github.com/adams-shaun/gorge/botpolicy"
	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/seat"
	"github.com/adams-shaun/gorge/view"
)

// Wrap decorates inner with the guard. The wrapper keeps inner's surfaces: a
// BoardSeat inner stays a BoardSeat (the host still hands it the Board, never a
// View), an EnvSeat inner stays an EnvSeat and RefusalAnswerer, and the payment
// opt-in is delegated. It also opts the seat into the engine's target hints.
// The wrapped seat is asked exactly once per decision, about the filtered
// option list.
func Wrap(inner seat.Seat) seat.Seat {
	base := guarded{inner: inner}
	if _, ok := inner.(seat.BoardSeat); ok {
		return guardedBoard{base}
	}
	if es, ok := inner.(bots.EnvSeat); ok {
		if ra, ok := inner.(bots.RefusalAnswerer); ok {
			return guardedEnv{base, es, ra}
		}
	}
	return base
}

type guarded struct{ inner seat.Seat }

func (s guarded) UnwrapSeat() seat.Seat { return s.inner }

// WantsTargetHints asks the host for decision.Option.TargetHints.
func (s guarded) WantsTargetHints() bool { return true }

func (s guarded) WantsPaymentActions() bool {
	if c, ok := s.inner.(seat.PaymentPlanConsumer); ok {
		return c.WantsPaymentActions()
	}
	return false
}

func (s guarded) Decide(ctx context.Context, v view.View, d decision.Decision) (decision.Intent, error) {
	f := Filter(d)
	in, err := s.inner.Decide(ctx, v, f.D)
	if err != nil {
		return in, err
	}
	return f.MapBack(in), nil
}

type guardedBoard struct{ guarded }

func (s guardedBoard) DecideBoard(ctx context.Context, b botpolicy.Board, d decision.Decision) (decision.Intent, error) {
	f := Filter(d)
	in, err := s.inner.(seat.BoardSeat).DecideBoard(ctx, b, f.D)
	if err != nil {
		return in, err
	}
	return f.MapBack(in), nil
}

type guardedEnv struct {
	guarded
	es bots.EnvSeat
	ra bots.RefusalAnswerer
}

func (s guardedEnv) WantsEnv(d *decision.Decision) bool { return s.es.WantsEnv(d) }

func (s guardedEnv) DecideEnv(ctx context.Context, env bots.Env, d decision.Decision) (decision.Intent, error) {
	f := Filter(d)
	in, err := s.es.DecideEnv(ctx, env, f.D)
	if err != nil {
		return in, err
	}
	return f.MapBack(in), nil
}

func (s guardedEnv) AnswerRefused(v view.View, d decision.Decision, refused decision.Intent) decision.Intent {
	f := Filter(d)
	return f.MapBack(s.ra.AnswerRefused(v, f.D, f.MapIn(refused)))
}
