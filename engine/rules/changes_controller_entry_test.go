package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// A card in a graveyard has no controller (CR 108.4) and enters directly under
// the resolving player's control (CR 110.2a), so Rise of the Dark Realms
// returning p1's creature under p0's control is not "an opponent gains control
// of a permanent from you" (CR 603.2): Zidane must stay silent.
const changesControllerEntryScenario = `{"name":"changes-controller-gaincontrol-entry","cr":["110.2a","603.2","108.4"],"why":"Rise returns p1's graveyard creature under p0; Zidane's ChangesController row must not fire",
 "setup":{"p0":{"hand":["Rise of the Dark Realms"]},"p1":{"battlefield":["Zidane, Tantalus Thief"],"graveyard":["Grizzly Bears"]}},
 "steps":[{"op":"cast","seat":0,"card":"p0:Rise of the Dark Realms","mana":"BBCCCCCCC"},{"op":"resolve"}]}`

func TestChangesControllerIgnoresGainControlEntryFromGraveyard(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(changesControllerEntryScenario))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("precondition: scenario did not play cleanly: %v", res.Fails)
	}
	last := res.Snapshots[len(res.Snapshots)-1]
	taken, zidane := false, false
	for _, p := range last.Permanents {
		taken = taken || (p.Controller == 0 && p.Name == "Grizzly Bears")
		zidane = zidane || (p.Controller == 1 && strings.HasPrefix(p.Name, "Zidane"))
	}
	if !taken {
		t.Fatalf("precondition: p0 never gained the bears: %+v", last.Permanents)
	}
	if !zidane {
		t.Fatalf("precondition: Zidane left the battlefield: %+v", last.Permanents)
	}
	for _, s := range res.Snapshots {
		for _, e := range s.Stack {
			if e.Kind == "ability" && strings.Contains(strings.ToLower(e.Source), "zidane") {
				t.Fatalf("Zidane's trigger fired on a graveyard entry: stack=%v", s.Stack)
			}
		}
	}
	for _, p := range last.Permanents {
		if p.Token && strings.Contains(strings.ToLower(p.Name), "treasure") {
			t.Fatalf("Treasure was created: %+v", last.Permanents)
		}
	}
}

// The Effect-created registration mirrors the printed matcher: the pre-entry
// control change of a graveyard card does not fire it, a battlefield
// permanent's control change (Threaten) still does.
func TestEffectChangesControllerIgnoresOffBattlefieldControlChange(t *testing.T) {
	t.Parallel()
	promise := card(t, "Name:ControlPromise\nManaCost:U\nTypes:Sorcery\n"+
		"A:SP$ Effect | Triggers$ TrigControl\n"+
		"SVar:TrigControl:Mode$ ChangesController | ValidCard$ Creature | ValidOriginalController$ You | TriggerZones$ Command | Execute$ TrigPain\n"+
		"SVar:TrigPain:DB$ LoseLife | Defined$ You | LifeAmount$ 2\nOracle:x\n")
	e := handEngine(t, promise)
	e.G.Players[0].Pool[state.MU] = 1
	e.askPriority(0)
	castFirst(t, e, "cast")
	passUntilStackEmpty(t, e, 8)
	if len(e.G.Delayed) != 1 || e.G.Delayed[0].EventMode != "ChangesController" {
		t.Fatalf("precondition: ChangesController was not armed: %+v", e.G.Delayed)
	}
	const bears = "Name:Bear\nTypes:Creature Bear\nPT:2/2\nOracle:x\n"
	grave := inZoneCard(t, e, 0, state.ZGraveyard, bears)
	live := onBoard(t, e, 0, bears)
	if o := e.G.Obj(grave); o == nil || o.Zone != state.ZGraveyard || o.Controller != 0 {
		t.Fatalf("precondition: graveyard card misplaced: %+v", o)
	}
	if o := e.G.Obj(live); o == nil || o.Zone != state.ZBattlefield || o.Controller != 0 {
		t.Fatalf("precondition: permanent misplaced: %+v", o)
	}
	before := e.G.Players[0].Life
	e.emit(events.Event{Kind: events.ControlChange, Obj: grave, Player: 1, Text: "GainControl"})
	e.putTriggersOnStack()
	passUntilStackEmpty(t, e, 8)
	if got := e.G.Players[0].Life; got != before {
		t.Fatalf("off-battlefield control change fired: life %d, want %d", got, before)
	}
	e.emit(events.Event{Kind: events.ControlChange, Obj: live, Player: 1})
	e.putTriggersOnStack()
	passUntilStackEmpty(t, e, 8)
	if got, want := e.G.Players[0].Life, before-2; got != want {
		t.Fatalf("battlefield control change life = %d, want %d", got, want)
	}
}
