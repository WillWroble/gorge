package host

import (
	"context"
	"testing"

	"github.com/adams-shaun/gorge/bots/heuristic"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/sanity"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/view"
)

// The host's human-seat caretaker is built with SkipLifePlans (spec §6): for
// every hosted policy, offered only a life-paying plan, it answers from the
// ordinary options, while the same policy's hosted bot submits the plan.
func TestCaretakerNeverSelectsLifePayingPlan(t *testing.T) {
	d, v := lifeOnlyDecision()
	for _, policy := range []string{BotPolicy, LethalPressurePolicy, CastProfilePolicy} {
		t.Run(policy, func(t *testing.T) {
			caretaker, err := newCaretakerSeat(policy, 19, true, false, bots.Deps{})
			if err != nil {
				t.Fatal(err)
			}
			in, err := caretaker.Decide(context.Background(), v, d)
			if err != nil {
				t.Fatal(err)
			}
			if in.Payment != nil {
				t.Fatalf("caretaker submitted the life-paying plan: %+v", in.Payment)
			}
			if err := d.Validate(in); err != nil {
				t.Fatalf("caretaker intent %+v invalid: %v", in, err)
			}
			bot, err := NewBotPolicySeatWithAutoPayMana(policy, 19, true)
			if err != nil {
				t.Fatal(err)
			}
			in, err = bot.Decide(context.Background(), v, d)
			if err != nil {
				t.Fatal(err)
			}
			if in.Payment == nil || in.Payment.Plan.ID != "plan" {
				t.Fatalf("hosted bot intent = %+v, want the life-paying plan", in)
			}
		})
	}
	// Without auto-pay the caretaker is the plain policy bot.
	s, err := newCaretakerSeat(BotPolicy, 19, false, false, bots.Deps{})
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := s.(*heuristic.Bot); !ok || b.WantsPaymentActions() {
		t.Fatalf("caretaker without auto-pay = %T wants payment actions", s)
	}
}

// The guard decorator must not hide the *heuristic.Bot from the caretaker's
// SkipLifePlans arming (it exposes UnwrapSeat).
func TestCaretakerSkipLifePlansSeesThroughGuard(t *testing.T) {
	b := heuristic.NewBot(19).EnableAutoPayMana()
	skipLifePlans(sanity.Wrap(b), true)
	d, v := lifeOnlyDecision()
	in, err := b.Decide(context.Background(), v, d)
	if err != nil {
		t.Fatal(err)
	}
	if in.Payment != nil {
		t.Fatalf("guarded caretaker submitted the life-paying plan: %+v", in.Payment)
	}
}

func lifeOnlyDecision() (decision.Decision, view.View) {
	d := decision.Decision{Seq: 1, Player: 0, Kind: decision.KPriority, Min: 1, Max: 1,
		Options: []decision.Option{{Index: 0, Kind: "activate", Obj: 5}, {Index: 1, Kind: "pass"}},
		PaymentActions: []decision.PaymentAction{{ID: "action", Cast: decision.PlannedCast{Object: 9, Origin: "hand"},
			Plans: []decision.PaymentPlan{{ID: "plan", Version: decision.PaymentPlanV1, Activations: []decision.PaymentActivation{{
				Source: 5, Ability: decision.PaymentAbility{Kind: decision.PaymentAbilityPrinted},
				Produces: decision.ManaAmount{0, 0, 0, 0, 1, 0}, Consequence: &decision.PaymentConsequence{Life: 1}}}}}}},
	}
	v := view.View{Phase: "main1", Viewer: 0, Active: 0,
		Players: []view.PlayerView{{ID: 0, Hand: []view.CardView{{ID: 9, Types: "Creature", Power: 3, ManaCost: "2 G"}}}}}
	return d, v
}
