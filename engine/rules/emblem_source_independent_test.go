package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/state"
)

// CR 114.4: an emblem's abilities function in the command zone and the
// emblem is independent of the object that created it. Kaito, Cunning
// Infiltrator's [-9] emblem ("Whenever a player casts a spell, you create a
// 2/1 blue Ninja creature token") used to be registered with the
// battlefield-source bound a Duration$ Permanent Effect from a permanent
// carries, so it died with Kaito -- usually at once, since the [-9] itself
// drops Kaito to zero loyalty.
func TestKaitoEmblemOutlivesKaito(t *testing.T) {
	e, _ := fdnCensusEngine(t, "Kaito, Cunning Infiltrator", "Grizzly Bears")
	kaito := moveByName(t, e, 0, "Kaito, Cunning Infiltrator", state.ZBattlefield)
	bears := moveByName(t, e, 0, "Grizzly Bears", state.ZHand)
	f := e.G.Obj(kaito).Face()
	var ult *cards.SA
	for _, sa := range f.Abilities {
		if sa.ParamStr(cards.PKName) != "" {
			ult = sa
		}
	}
	if ult == nil {
		t.Fatal("no emblem ability on Kaito")
	}
	before := len(e.G.Delayed)
	effects.Resolve(e, &effects.Ctx{Source: kaito, Controller: 0, SVars: f.SVars}, ult)
	if len(e.G.Delayed) != before+1 {
		t.Fatalf("emblem registered %d delayed triggers, want 1", len(e.G.Delayed)-before)
	}
	if e.G.Delayed[before].SourceBattlefield {
		t.Fatal("Kaito's emblem is bounded by Kaito's battlefield presence")
	}
	e.emit(events.Event{Kind: events.MoveZone, Obj: kaito, From: state.ZBattlefield, To: state.ZGraveyard})
	if len(e.G.Delayed) != before+1 || !e.delayedRegistrationLive(&e.G.Delayed[before]) {
		t.Fatalf("Kaito's emblem ended when Kaito left the battlefield: %+v", e.G.Delayed)
	}
	addMana(t, e, 0, "GC")
	castByID(t, e, bears)
	fdnCensusDrain(t, e)
	ninjas := 0
	for _, id := range e.G.Zone(state.ZBattlefield, 0) {
		if o := e.G.Obj(id); o.IsToken && o.Face() != nil && o.Face().Name == "Ninja Token" {
			ninjas++
		}
	}
	if ninjas != 1 {
		t.Fatalf("Ninja tokens after a cast with Kaito gone = %d, want 1", ninjas)
	}
}
