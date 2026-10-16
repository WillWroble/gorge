package rules

// Brigid, Doun's Mind: "{T}: Add X {G} or X {W}" is Produced$ Combo G W |
// Each$ X. Each$ multiplies the one chosen colour's units; at X = 0 nothing is
// added and no colour is asked (CR 106.1 / 605.3b).

import (
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/internal/testutil"
)

func eachSource(x string) string {
	return "Name:Each Land\nTypes:Land\n" +
		"A:AB$ Mana | Cost$ T | Produced$ Combo G W | Each$ X | SpellDescription$ Add X.\n" +
		"SVar:X:Number$" + x + "\nOracle:x\n"
}

func TestManaEachMultipliesTheChosenColour(t *testing.T) {
	t.Parallel()
	e, cfg, id := manaSourceEngine(t, eachSource("3"))
	activateMana(t, e, id)
	d := e.Pending()
	if d == nil || len(d.Options) != 2 {
		t.Fatalf("Each$ 3 must still ask G/W, pending = %+v", d)
	}
	submitChoices(t, e, manaOption(t, d, "W"))
	pool := e.G.Players[0].Pool
	if pool[state.MW] != 3 || pool.Total() != 3 {
		t.Fatalf("Each$ X=3 pool = %v, want exactly WWW", pool)
	}
	replayCheck(t, e, cfg)
}

func TestManaEachZeroAddsNothingAndAsksNothing(t *testing.T) {
	t.Parallel()
	e, cfg, id := manaSourceEngine(t, eachSource("0"))
	activateMana(t, e, id)
	if d := e.Pending(); d != nil && len(d.Options) == 2 && strings.Contains(d.Options[0].Label, "Add") {
		t.Fatalf("Each$ X=0 posed a colour ask: %+v", d)
	}
	if n := e.G.Players[0].Pool.Total(); n != 0 {
		t.Fatalf("Each$ X=0 pool = %v, want empty", e.G.Players[0].Pool)
	}
	for _, ev := range e.L.Events {
		if ev.Kind == events.Note && strings.Contains(ev.Text, "Each") {
			t.Fatalf("Each$ reported unread: %q", ev.Text)
		}
	}
	replayCheck(t, e, cfg)
}

// TestManaEachIsReadCorpusWide is the census ratchet: every api:Mana ability
// carrying Each$ compiles it (no Unread report). The set is pinned so a new
// carrier is noticed and given a test.
func TestManaEachIsReadCorpusWide(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	var names []string
	for _, c := range reg.AllCards() {
		for _, f := range c.Faces {
			for _, sa := range f.Abilities {
				if sa == nil || sa.API != "Mana" {
					continue
				}
				if _, has := sa.Params["Each"]; !has {
					continue
				}
				names = append(names, f.Name)
				mp := effects.ManaOf(sa)
				if !mp.Each.Present {
					t.Errorf("%s: Each$ present but not compiled", f.Name)
				}
				for _, k := range mp.Unread {
					if k == "Each" {
						t.Errorf("%s: Each$ reported unread", f.Name)
					}
				}
			}
		}
	}
	if len(names) < 1 {
		t.Fatalf("precondition: no corpus Mana ability carries Each$ (corpus missing?)")
	}
	sort.Strings(names)
	if got := strings.Join(names, "|"); got != "Brigid, Doun's Mind" {
		t.Fatalf("api:Mana Each$ ability carriers = %q; a new carrier needs its own test", got)
	}
}
