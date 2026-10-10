package main

import (
	"testing"

	"github.com/adams-shaun/gorge/seat"
)

type unwrapper interface{ UnwrapSeat() seat.Seat }

func TestSanityPoliciesAreOptInAndPlainNamesUnchanged(t *testing.T) {
	for _, base := range []string{"bot", "bot-auto-pay"} {
		if _, ok := policies[base](1).(unwrapper); ok {
			t.Errorf("%s is wrapped without opting in", base)
		}
		g, ok := policies[base+sanitySuffix]
		if !ok {
			t.Fatalf("%s%s not registered", base, sanitySuffix)
		}
		s := g(1)
		u, ok := s.(unwrapper)
		if !ok {
			t.Fatalf("%s+sanity = %T, not wrapped", base, s)
		}
		if _, ok := u.UnwrapSeat().(*seat.Bot); !ok {
			t.Errorf("%s+sanity inner = %T", base, u.UnwrapSeat())
		}
	}
	if !isAutoPayPolicy("bot-auto-pay+sanity") || isAutoPayPolicy("bot+sanity") {
		t.Error("isAutoPayPolicy must see through the +sanity suffix")
	}
	if _, ok := policies["sb-tactical+sanity"]; !ok {
		t.Error("sb-tactical+sanity not registered")
	}
	for _, n := range []string{"lethal-pressure+sanity", "sb-heuristic+sanity"} {
		if _, ok := policies[n]; ok {
			t.Errorf("%s must not exist", n)
		}
	}
}

func TestApplyCommonSenseRebindsPlainNames(t *testing.T) {
	saved := policies["bot"]
	defer func() { policies["bot"] = saved }()
	applyCommonSense()
	if _, ok := policies["bot"](1).(unwrapper); !ok {
		t.Fatal("-common-sense did not wrap bot")
	}
}
