package setdata_test

import (
	"testing"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/setdata"
)

func mustLoad(t *testing.T, code string) setdata.Spec {
	t.Helper()
	spec, err := setdata.Load(code)
	if err != nil {
		t.Fatalf("Load(%s): %v", code, err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("Load(%s): embedded spec fails its own validator: %v", code, err)
	}
	return spec
}

// sheetCounts returns each sheet's size.
func sheetCounts(spec setdata.Spec) map[string]int {
	out := map[string]int{}
	for name, cards := range spec.Sheets {
		out[name] = len(cards)
	}
	return out
}

func assertCounts(t *testing.T, spec setdata.Spec, want map[string]int) {
	t.Helper()
	got := sheetCounts(spec)
	for _, sheet := range []string{"common", "uncommon", "rare", "mythic", "land"} {
		if got[sheet] != want[sheet] {
			t.Errorf("%s sheet %s has %d cards, want %d", spec.Code, sheet, got[sheet], want[sheet])
		}
	}
}

// TestSetdataCodesAreExactlyFDNAndBLB pins the loader's universe: exactly
// the two embedded sets, sorted, and unknown or hostile codes refuse.
func TestSetdataCodesAreExactlyFDNAndBLB(t *testing.T) {
	codes := setdata.Codes()
	if len(codes) != 2 || codes[0] != "BLB" || codes[1] != "FDN" {
		t.Fatalf("setdata.Codes() = %v, want [BLB FDN]", codes)
	}
	if _, err := setdata.Load("NOPE"); err == nil {
		t.Error("Load(NOP) succeeded for an unembedded set")
	}
	if _, err := setdata.Load(""); err == nil {
		t.Error("Load(\"\") succeeded")
	}
	if _, err := setdata.Load("../headers/FDN"); err == nil {
		t.Error("Load accepted a path-traversal code")
	}
}

// TestSetdataFDNSpec matches the measured Foundations booster: 80
// pack-relevant commons after the ten gain lands move to the land sheet
// (Evolving Wilds stays a common), 101 uncommons, 60 rares, 20 mythics, and
// a 14-card slot recipe.
func TestSetdataFDNSpec(t *testing.T) {
	spec := mustLoad(t, "FDN")
	if spec.Name != "Foundations" || spec.PackSize != 14 || spec.MainMax != 271 {
		t.Errorf("FDN header fields = %q pack %d main_max %d", spec.Name, spec.PackSize, spec.MainMax)
	}
	assertCounts(t, spec, map[string]int{"common": 80, "uncommon": 101, "rare": 60, "mythic": 20, "land": 10})
	if got := findCard(t, spec, "common", "Evolving Wilds"); got == nil {
		t.Error("Evolving Wilds should stay on the common sheet")
	}
	if got := findCard(t, spec, "common", "Bloodfell Caves"); got != nil {
		t.Error("Bloodfell Caves should have moved off the common sheet to the land sheet")
	}
	if got := findCard(t, spec, "land", "Tranquil Cove"); got == nil {
		t.Error("Tranquil Cove missing from the land sheet")
	} else if got.Rarity != limited.Land || got.Number != "270" {
		t.Errorf("land-sheet Tranquil Cove = %+v, want rarity L number 270", *got)
	}
}

// TestSetdataBLBSpec matches the measured Bloomburrow booster: 81
// commons, 99 uncommons, 60 rares, 20 mythics, and the five basics on the
// land sheet carrying their lowest collector number (262-278, outside the
// main runs).
func TestSetdataBLBSpec(t *testing.T) {
	spec := mustLoad(t, "BLB")
	if spec.Name != "Bloomburrow" || spec.PackSize != 14 || spec.MainMax != 261 {
		t.Errorf("BLB header fields = %q pack %d main_max %d", spec.Name, spec.PackSize, spec.MainMax)
	}
	assertCounts(t, spec, map[string]int{"common": 81, "uncommon": 99, "rare": 60, "mythic": 20, "land": 5})
	if got := findCard(t, spec, "land", "Plains"); got == nil {
		t.Error("Plains missing from the land sheet")
	} else if got.Rarity != limited.Land || got.Number != "262" {
		t.Errorf("land-sheet Plains = %+v, want rarity L number 262", *got)
	}
	if got := findCard(t, spec, "common", "Blossoming Sands"); got != nil {
		t.Error("Blossoming Sands is a bonus-sheet gain land and must not be pack material")
	}
}

// findCard returns the sheet card with the given name, or nil.
func findCard(t *testing.T, spec setdata.Spec, sheet, name string) *setdata.SheetCard {
	t.Helper()
	for _, c := range spec.Sheets[sheet] {
		if c.Name == name {
			return &c
		}
	}
	return nil
}

// TestSetdataValidateAcceptsAndRejects exercises the validator's rules on
// broken copies of a known-good spec; the loaded spec passing Validate
// first is what makes the rejections meaningful.
func TestSetdataValidateAcceptsAndRejects(t *testing.T) {
	good := mustLoad(t, "BLB")
	cases := map[string]func(s *setdata.Spec){
		"slot counts overfill the pack": func(s *setdata.Spec) { s.Slots[0].Count++ },
		"empty sheet":                   func(s *setdata.Spec) { s.Sheets["land"] = nil },
		"wildcard weights sum to 0.9": func(s *setdata.Spec) {
			for i := range s.Slots {
				if w := s.Slots[i].Wildcard; w != nil {
					w.Common = 0.571 - 0.1
				}
			}
		},
		"card on two sheets": func(s *setdata.Spec) {
			s.Sheets["land"] = append(s.Sheets["land"], s.Sheets["common"][0])
		},
		"name twice on one sheet": func(s *setdata.Spec) {
			dup := s.Sheets["common"][0]
			s.Sheets["common"] = append(s.Sheets["common"], dup)
		},
		"slot names a missing sheet": func(s *setdata.Spec) { s.Slots[0].Sheet = "unreal" },
		"slot sets two kinds":        func(s *setdata.Spec) { s.Slots[0].RareMythic = &setdata.RareMythic{RareWeight: 1, MythicWeight: 1} },
		"zero-count slot":            func(s *setdata.Spec) { s.Slots[1].Count = 0 },
	}
	for name, breakIt := range cases {
		s := good
		breakIt(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("Validate accepted a spec with %s", name)
		}
	}
}
