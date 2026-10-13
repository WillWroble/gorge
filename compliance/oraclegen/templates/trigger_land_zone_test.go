package templates

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/compliance/levelb"
	"github.com/adams-shaun/gorge/compliance/oraclegen"
)

// TestLandPlayedZoneRowsGenerate is the class table for the origin-scoped
// LandPlayed trigger (wave-4 G5 LandPlayed rows): each row generates, plays
// through gorge, plays its land from a non-hand zone, and shows the row's own
// trigger on the stack.
func TestLandPlayedZoneRowsGenerate(t *testing.T) {
	reg := loadGenRegistry(t)
	for _, tc := range []struct{ name, key string }{
		{"Gwen Stacy", "trigger#1.0"},
		{"Shadow of the Goblin", "trigger#0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := triggerRequirement(t, reg, tc.name, tc.key, levelb.LandPlayedZoneSub)
			if res, ok := oraclegen.PlaysThrough(reg, it.Scenario); !ok || len(res.Fails) != 0 {
				t.Fatalf("does not play through gorge: ok=%v fails=%v", ok, res.Fails)
			}
			played := false
			for _, st := range it.Scenario.Steps {
				played = played || st.Op == "play"
			}
			if !played {
				t.Fatalf("no play step: %+v", it.Scenario.Steps)
			}
			slot := tc.key[strings.LastIndexByte(tc.key, '.')+1:]
			c, _ := reg.Lookup(tc.name)
			face := c.Faces[0]
			if strings.HasPrefix(tc.key, "trigger#1.") {
				face = c.Faces[1]
			}
			res, _ := oraclegen.PlaysThrough(reg, it.Scenario)
			if !abilityOnStack(res.Snapshots, stackSourceWants(reg, tc.name, face), slot) {
				t.Fatalf("no snapshot shows trigger %s on the stack", slot)
			}
		})
	}
}

// TestChosenCardLeavesRowGenerates: Zenos yae Galvus's "When the chosen
// creature leaves the battlefield" is served by casting Zenos (its ETB
// chooses p1's creature) and removing that creature; the row's trigger
// reaches the stack.
func TestChosenCardLeavesRowGenerates(t *testing.T) {
	reg := loadGenRegistry(t)
	it := triggerRequirement(t, reg, "Zenos yae Galvus", "trigger#0.1", "trigger.zone-change-residue")
	res, ok := oraclegen.PlaysThrough(reg, it.Scenario)
	if !ok || len(res.Fails) != 0 {
		t.Fatalf("does not play through gorge: ok=%v fails=%v", ok, res.Fails)
	}
	// The trigger's body transforms Zenos: the resolved scenario shows its
	// back face, which only the row's own trigger produces.
	for _, p := range res.Snapshots[len(res.Snapshots)-1].Permanents {
		if p.Name == "Shinryu, Transcendent Rival" {
			return
		}
	}
	t.Fatalf("Zenos did not transform: %+v", res.Snapshots[len(res.Snapshots)-1].Permanents)
}

// TestStateCounterDoublingRowNamedSkip: The Millennium Calendar's state
// trigger keeps a named skip -- its doubling cause ends the game, which the
// XMage driver cannot replay past.
func TestStateCounterDoublingRowNamedSkip(t *testing.T) {
	reg := loadGenRegistry(t)
	c, _ := reg.Lookup("The Millennium Calendar")
	for _, r := range levelb.Requirements(c) {
		if r.Key != "trigger#0.1" {
			continue
		}
		_, skip := GenerateB(reg, "The Millennium Calendar", r)
		if skip == nil || !strings.Contains(skip.Reason, "ends the game") {
			t.Fatalf("skip = %v, want the game-over named skip", skip)
		}
		return
	}
	t.Fatal("no trigger#0.1")
}

// TestAttachedRelativeBearerRowGenerates: Eriette, the Beguiler's
// "Whenever an Aura you control becomes attached to a nonland permanent an
// opponent controls with mana value <= that Aura's" is caused by Pacifism on
// p1's Grizzly Bears; its gain-control effect takes the Bears.
func TestAttachedRelativeBearerRowGenerates(t *testing.T) {
	reg := loadGenRegistry(t)
	it := triggerRequirement(t, reg, "Eriette, the Beguiler", "trigger#0.0", "trigger.attached")
	res, ok := oraclegen.PlaysThrough(reg, it.Scenario)
	if !ok || len(res.Fails) != 0 {
		t.Fatalf("does not play through gorge: ok=%v fails=%v", ok, res.Fails)
	}
	for _, p := range res.Snapshots[len(res.Snapshots)-1].Permanents {
		if p.Name == "Grizzly Bears" && p.Owner == 1 && p.Controller == 0 {
			return
		}
	}
	t.Fatalf("p0 did not gain p1's Grizzly Bears: %+v", res.Snapshots[len(res.Snapshots)-1].Permanents)
}
