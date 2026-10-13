// Class G12 (wave 4, "trigger did not fire"): the recipe caused the right
// trigger mode but the trigger's filter or condition rejected the cause. One
// row per census row (.ds4/levelb-census/wave4 G12); a served row must
// generate and show its own trigger on the stack, and a residue row is pinned
// to its skip reason, so a regression and an improvement both fail here.
package templates

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/compliance"

	"github.com/adams-shaun/gorge/compliance/levelb"
	"github.com/adams-shaun/gorge/compliance/oraclegen"
)

func TestG12TriggerFilterRows(t *testing.T) {
	reg := loadGenRegistry(t)
	// Generate as oraclediff gen does: only probes XMage's database holds.
	installManifestXMageKnown(t)
	const served = ""
	for _, tc := range []struct{ name, key, skip string }{
		// Setup's turn-1 upkeep spent the target or the source: cast on turn 1.
		{"Virtue of Persistence", "trigger#0.0", served},
		{"Undergrowth Recon", "trigger#0.0", served},
		{"Mistbreath Elder", "trigger#0.0", served},
		// An unattached Aura: cast onto a bearer.
		{"Chorale of the Void", "trigger#0.1", served},
		// A printed x/0 source: a +1/+1 counter keeps it alive.
		{"Wildgrowth Archaic", "trigger#0.0", served},
		// Damage thresholds.
		{"Innocent Bystander", "trigger#0.0", served},
		{"Spinneret and Spiderling", "trigger#0.1", served},
		// A crime condition.
		{"Servant of the Stinger", "trigger#0.0", served},
		// Target filters the fixtures now place (NotDefined*, equipped,
		// modified).
		{"Jackdaw Savior", "trigger#0.0", served},
		{"Pawpatch Recruit", "trigger#0.0", served},
		{"Item Shopkeep", "trigger#0.0", served},
		{"Silver Sable, Mercenary Leader", "trigger#0.1", served},
		// Spell-cast filters.
		{"Jackal, Genius Geneticist", "trigger#0.0", served},
		{"Tokka & Rahzar, Terrible Twos", "trigger#0.0", served},
		{"Fin Fang Foom", "trigger#0.0", served},
		{"Riku of Many Paths", "trigger#0.0", served},
		{"Danitha, Sword of Hope", "trigger#0.0", served},
		// Origin zones.
		{"Oscorp Industries", "trigger#0.0", served},
		{"Fire Lord Zuko", "trigger#0.1", served},
		// A graveyard trigger whose sibling would return the card first.
		{"Fear of Infinity", "trigger#0.1", served},
		// Residue (see the lb4-g12 report).
		{"Coveted Falcon", "trigger#0.0", "trigger did not fire"},                                                                // "permanent you own but don't control": no setup gives p1 control of a p0-owned permanent
		{"Oblivious Bookworm", "trigger#0.2", "trigger did not fire"},                                                            // Static$ True bookkeeping trigger: never uses the stack
		{"Mikey & Don, Party Planners", "trigger#0.0", "trigger mayplay rider: the permission scenario ends in no offered cast"}, // the MayPlay rider recipe (lb4-gtrig) needs a Mutant/Ninja/Turtle creature probe; the static's offered probe is a land play
		{"Avatar Aang", "trigger#1.0", "trigger did not fire"},                                                                   // back face transforms away during setup's turn-1 upkeep
		{"Codie, Ravenous Codex", "trigger#0.0", "trigger did not fire"},                                                         // the prepared-copy probe's spell face (Whiplash Wordsmith's) is not in the XMage manifests
		{"Smaug, Wicked Worm", "trigger#0.1", "trigger did not fire"},                                                            // Spell.ManaFromTreasure: the scenario pays from a seeded pool, never a Treasure
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, ok := reg.Lookup(tc.name)
			if !ok {
				t.Fatalf("%s not in the corpus", tc.name)
			}
			var req *levelb.Requirement
			for _, r := range levelb.Requirements(c) {
				if r.Key == tc.key {
					r := r
					req = &r
				}
			}
			if req == nil {
				t.Fatalf("%s has no requirement %s", tc.name, tc.key)
			}
			it, skip := GenerateB(reg, tc.name, *req)
			if tc.skip != served {
				if skip == nil || skip.Reason != tc.skip {
					t.Fatalf("residue row moved: skip = %+v, want %q (update the row and the report)", skip, tc.skip)
				}
				return
			}
			if skip != nil {
				t.Fatalf("not served: %s", skip.Reason)
			}
			res, ok := oraclegen.PlaysThrough(reg, it.Scenario)
			if !ok || len(res.Fails) != 0 {
				t.Fatalf("does not play through gorge: ok=%v fails=%v", ok, res.Fails)
			}
			slot := tc.key[strings.LastIndexByte(tc.key, '.')+1:]
			probe := it.Scenario
			probe.Steps = append([]oraclegen.Step(nil), it.Scenario.Steps...)
			for i := range probe.Steps {
				// A combat-damage trigger resolves inside the pass to main2;
				// the recipe's own probe stops in end-combat, where it is
				// still on the stack.
				if probe.Steps[i].Op == "pass_to" && probe.Steps[i].Step == "main2" {
					probe.Steps[i].Step = "end-combat"
				}
			}
			if !g17SlotOnStack(t, reg, probe, tc.name, slot) {
				t.Fatalf("no snapshot shows trigger %s on the stack sourced by %s", slot, tc.name)
			}
		})
	}
}

// installManifestXMageKnown loads every committed set manifest's names into
// oraclegen.XMageKnown, the filter oraclediff gen runs under, and restores
// the permissive default afterwards.
func installManifestXMageKnown(t *testing.T) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "manifests", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no committed manifests found (glob err %v, %d paths)", err, len(paths))
	}
	var names []string
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var m compliance.Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		for _, c := range m.Cards {
			names = append(names, c.Name)
		}
	}
	oraclegen.SetXMageKnown(names)
	t.Cleanup(func() { oraclegen.SetXMageKnown(nil) })
}
