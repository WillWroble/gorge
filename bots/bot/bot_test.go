package bot

import (
	"testing"

	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/seat"
)

type unwrapper interface{ UnwrapSeat() seat.Seat }

func TestCommonSenseOffReturnsThePlainSeat(t *testing.T) {
	s, err := bots.New("bot", bots.Options{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*seat.Bot); !ok {
		t.Fatalf("manual bot = %T, want *seat.Bot", s)
	}
	s, err = bots.New("bot", bots.Options{Seed: 1, AutoPayMana: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*seat.Bot); !ok {
		t.Fatalf("auto-pay bot = %T, want *seat.Bot", s)
	}
}

func TestCommonSenseOnWrapsAndKeepsSurfaces(t *testing.T) {
	for _, auto := range []bool{false, true} {
		s, err := bots.New("bot", bots.Options{Seed: 1, AutoPayMana: auto, CommonSense: true})
		if err != nil {
			t.Fatal(err)
		}
		u, ok := s.(unwrapper)
		if !ok {
			t.Fatalf("auto=%v: %T is not wrapped", auto, s)
		}
		if _, ok := u.UnwrapSeat().(*seat.Bot); !ok {
			t.Fatalf("auto=%v: inner = %T, want *seat.Bot", auto, u.UnwrapSeat())
		}
		if c, ok := s.(seat.TargetHintConsumer); !ok || !c.WantsTargetHints() {
			t.Fatalf("auto=%v: wrapper does not request target hints", auto)
		}
		pc, ok := s.(seat.PaymentPlanConsumer)
		if !ok || pc.WantsPaymentActions() != auto {
			t.Fatalf("auto=%v: payment-plan opt-in not delegated (%v)", auto, ok)
		}
	}
}

func TestCommonSenseIgnoredByOtherPolicies(t *testing.T) {
	for _, name := range []string{"lethal-pressure", "sb-uniform", "sb-first"} {
		s, err := bots.New(name, bots.Options{Seed: 1, AutoPayMana: true, CommonSense: true})
		if err != nil {
			continue // needs deps; not wrapped either way
		}
		if _, ok := s.(unwrapper); ok {
			t.Errorf("%s was wrapped; CommonSense is for bot and sb-tactical only", name)
		}
	}
}
