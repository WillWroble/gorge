package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// The oracle runner records every canonical coin-flip result the seeded RNG
// produced, so the compliance generator can script each one for XMage's
// choice queue ("AssertionError: Missing FLIP COIN RESULT def for turn 1,
// step UPKEEP, PlayerA"). Mana Crypt's trigger#0.0 scenario flips twice: the
// drive-window flip before turn 1's first step (lost), the upkeep trigger's
// resolution during turn 3 (won).
func TestOracleFlipRecord(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(`{"name":"flip-record","setup":{"p0":{"battlefield":["Mana Crypt"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[{"op":"pass_to","seat":0,"step":"upkeep","active":"p0"},{"op":"resolve","seat":0}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, joinLines(res.Transcript))
	}
	want := []OracleFlip{
		{Turn: 1, Step: -1, Seat: 0, Win: false},
		{Turn: 3, Step: 1, Seat: 0, Win: true},
	}
	if len(res.Flips) != len(want) {
		t.Fatalf("flips = %+v, want %+v", res.Flips, want)
	}
	for i, w := range want {
		if res.Flips[i] != w {
			t.Errorf("flip %d = %+v, want %+v", i, res.Flips[i], w)
		}
	}

	// A scenario that flips no coin records none, leaving its result bytes
	// unchanged (Shock's cast-resolve sha is pinned by the templates test).
	noFlip, err := RunOracleScenarioJSON(reg, []byte(`{"name":"no-flip","setup":{"p0":{"battlefield":["Grizzly Bears"]},"p1":{"battlefield":["Grizzly Bears"]}},"steps":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(noFlip.Fails) != 0 || len(noFlip.Flips) != 0 {
		t.Errorf("no-flip scenario: fails=%v flips=%+v, want none", noFlip.Fails, noFlip.Flips)
	}
}
