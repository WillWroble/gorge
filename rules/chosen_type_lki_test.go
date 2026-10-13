package rules

// CR 113.7a / 608.2b / 608.2h: a target filter naming the source's recorded
// "as this enters" choice (Card.ChosenType) is re-judged at resolution against
// the source's last-known choice when the source has left the battlefield
// (the live object's choices are cleared on the move) -- Dawn-Blessed
// Pennant, sacrificed as a cost, still returns a card of the chosen type.

import (
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

const chosenTypeElfSrc = "Name:LKIElf\nManaCost:G\nTypes:Creature Elf\nPT:1/1\nOracle:x\n"

func chosenTypePennantSrc(cost string) string {
	return "Name:LKIPennant\nManaCost:0\nTypes:Artifact\n" +
		"A:AB$ ChangeZone | Cost$ " + cost + " | TgtPrompt$ Select target card of the chosen type in your graveyard | " +
		"ValidTgts$ Card.ChosenType+YouOwn | Origin$ Graveyard | Destination$ Hand | " +
		"SpellDescription$ Return target card of the chosen type from your graveyard to your hand.\nOracle:x\n"
}

func TestChosenTypeTargetFilterReadsDepartedSourceLKI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, cost string
		destroy    bool
	}{
		{"destroyed-in-response", "0", true},
		{"sacrificed-as-cost", "Sac<1/CARDNAME>", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, cfg, id := newFixtureDeck(t, 23, chosenTypePennantSrc(tc.cost), chosenTypeElfSrc)
			moveByName(t, e, 0, "LKIPennant", state.ZBattlefield)
			elf := moveByName(t, e, 0, "LKIElf", state.ZGraveyard)
			e.G.Obj(id).ChosenType = "Elf"
			if z := e.G.Obj(id).Zone; z != state.ZBattlefield {
				t.Fatalf("precondition: source zone %v, want battlefield", z)
			}
			addMana(t, e, 0, "")
			submitChoices(t, e, abilityOption(t, e, id, 0).Index)
			answerTargetAsk(t, e, []state.ObjID{elf})
			if len(e.G.Stack) != 1 {
				t.Fatalf("activation did not reach the stack: %v", e.G.Stack)
			}
			if tc.destroy {
				e.emit(events.Event{Kind: events.MoveZone, Obj: id, From: state.ZBattlefield, To: state.ZGraveyard})
			}
			if o := e.G.Obj(id); o.Zone != state.ZGraveyard || o.ChosenType != "" {
				t.Fatalf("precondition: source zone %v ChosenType %q, want graveyard and cleared", o.Zone, o.ChosenType)
			}
			passUntilStackEmpty(t, e, 400)
			if z := e.G.Obj(elf).Zone; z != state.ZHand {
				t.Fatalf("target legal by the source's last-known chosen type (CR 608.2h); it is in zone %v", z)
			}
			if diff := diffGames(e.G, replayFromLog(t, cfg, e.L.Events)); diff != "" {
				t.Fatalf("log-only replay differs:\n%s", diff)
			}
		})
	}
}
