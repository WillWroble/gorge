package main

import (
	"testing"

	"github.com/adams-shaun/gorge/bots/azmcts"
	"github.com/adams-shaun/gorge/bots/searchprobe"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// :closedlist parses as an az key only, and the seat it builds once play has set
// the belief searches the closed world with that belief.
func TestParseClosedList(t *testing.T) {
	p, err := parsePolicy("az:sims=8:autopay:oppnodes:oppfull:morekinds:closedlist")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Closed {
		t.Fatal(":closedlist did not set Closed")
	}
	if _, err := parsePolicy("bot:closedlist"); err == nil {
		t.Fatal("bot:closedlist parsed")
	}
	if _, err := p.seat(1); err == nil {
		t.Fatal("a closed seat was built without a belief")
	}
	_, decks := testutil.SampleDecks(t, 2)
	b, err := searchprobe.NewBelief(decks)
	if err != nil {
		t.Fatal(err)
	}
	p.setBelief(b)
	cfg := p.azConfig()
	if cfg.World != azmcts.WorldRedealClosed || cfg.Belief != b {
		t.Fatalf("closed spec: world %q", cfg.World)
	}
	st, err := p.seat(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.(*azmcts.Seat).SearchBelief() != b {
		t.Fatal("the closed seat does not carry the belief")
	}
	open, err := parsePolicy("az:sims=8")
	if err != nil {
		t.Fatal(err)
	}
	open.setBelief(b)
	if open.azConfig().World != azmcts.WorldRedeal {
		t.Fatal("an open spec took the closed world")
	}
}
