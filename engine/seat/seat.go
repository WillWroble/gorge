// Package seat is who answers the engine's decisions. It sits above rules in
// the dependency order (cards -> state -> decision -> events -> effects ->
// rules -> view -> seat -> replay -> cmd/*): a Seat is handed a view.View,
// never a *state.Game or a *rules.Engine, so it only ever sees what a real
// client would.
package seat

import (
	"context"

	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/view"
)

// Seat is anything that answers decisions: a bot, a scripted test seat, or
// (later) a WebSocket-backed human. ctx lets a human seat time out; a bot
// ignores it and never returns an error (Ruling P8).
type Seat interface {
	Decide(ctx context.Context, v view.View, d decision.Decision) (decision.Intent, error)
}

// PaymentPlanConsumer opts into the lazy payment-action extension: a driver
// (testkit/bench.PlayGame, server/host/match.go, cmd tools) calls
// Engine.EnsurePaymentActions before a decision is handed to a seat that
// answers true, so Decision.PaymentActions carries the engine's mana-payment
// plans. A seat that does not implement it, or answers false, gets no plans.
//
// Bots answer true by default. A bot without plans pays by tapping mana by
// hand, and a search seat whose fallback bot taps sees that tap as an answer
// outside its candidates: az lost 89% of its counterspell windows that way
// (2026-10-14). So every bot implements this interface, and a wrapper
// forwards to what it wraps (or answers true). A bot opts out only with a
// stated reason: its policy reads the manual option list and never a plan
// (a manual-mana Bot, heuristic.PolicyNetBot, the searchteacher seat, the
// generation-0 az-redeal config). bots/all's
// TestRegisteredBotsConsumePaymentPlans enforces this for every registered
// policy, with the opt-outs named there.
//
// Human/client seats are not consumers: the host builds their plans itself
// (host.HumanSeat under TableConfig.AutoMana); the client only picks one, and
// the engine validates the pick.
type PaymentPlanConsumer interface {
	WantsPaymentActions() bool
}

// PotentialCastConsumer opts into the casts all of a seat's available mana
// can pay, not only those its floating pool pays (rules'
// Engine.EnsurePotentialCasts): mz-gorge's autotap player, which pays them
// while casting. A driver extends the seat's priority decisions for it the
// way it builds payment plans for a PaymentPlanConsumer. No other seat sees
// the additions.
type PotentialCastConsumer interface {
	WantsPotentialCasts() bool
}

// EngineSeat reads the whole game: a driver hands it a clone of the live
// engine at each of its decisions instead of a View. It is mz-gorge's
// perfect-information search (mz-gorge-design.md, principle 10), so only
// testkit/bench drives it; a hosted table never does, as it would show the
// seat every hidden card.
type EngineSeat interface {
	Seat
	DecideEngine(ctx context.Context, e *rules.Engine, d decision.Decision) (decision.Intent, error)
}

// TargetHintConsumer opts into decision.Option.TargetHints on priority
// decisions (Engine.EnsureTargetHints). Information only; the engine offers
// every legal action either way.
type TargetHintConsumer interface {
	WantsTargetHints() bool
}
