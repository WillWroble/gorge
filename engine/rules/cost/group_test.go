package cost

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
)

// TestParseGroupExileCost: the open-ended "N+" exile count with its
// withTotal* group predicate parses to one AtLeast Exile part whose spec no
// longer carries the predicate, and FormatCost spells it back.
func TestParseGroupExileCost(t *testing.T) {
	for _, tc := range []struct {
		tok   string
		zone  state.Zone
		spec  string
		group GroupTotal
	}{
		{"Exile<1+/Artifact.Other+withTotalCMCEQX/other artifacts you control with total mana value X>", state.ZBattlefield, "Artifact.Other",
			GroupTotal{Prop: GroupCMC, Cmp: GroupEQ, Var: "X", Raw: "withTotalCMCEQX"}},
		{"ExileFromGrave<1+/Card.Black+withTotalManaSymbols_Black_GE15>", state.ZGraveyard, "Card.Black",
			GroupTotal{Prop: GroupManaSymbols, Color: 'B', Cmp: GroupGE, N: 15, Raw: "withTotalManaSymbols_Black_GE15"}},
		{"ExileFromGrave<1+/Card.Historic+withTotalCMCGE30>", state.ZGraveyard, "Card.Historic",
			GroupTotal{Prop: GroupCMC, Cmp: GroupGE, N: 30, Raw: "withTotalCMCGE30"}},
	} {
		c := ParseCost("T " + tc.tok)
		if len(c.Unknown) != 0 || c.Generic != 0 || len(c.Exile) != 1 {
			t.Fatalf("%s parsed to %+v", tc.tok, c)
		}
		p := c.Exile[0]
		if !p.AtLeast || p.N != 1 || p.Zone != tc.zone || p.Spec != tc.spec || p.Group != tc.group {
			t.Errorf("%s part = %+v", tc.tok, p)
		}
		back := ParseCost(FormatCost(c))
		if !back.Equal(&Cost{Tap: true, Exile: []CostPart{{N: 1, Spec: tc.spec, Zone: tc.zone, AtLeast: true, Group: tc.group}}}) {
			t.Errorf("%s: FormatCost %q re-parses to %+v", tc.tok, FormatCost(c), back)
		}
	}
	// An unread withTotal predicate is reported, never left in the spec as a
	// term no object matches.
	if c := ParseCost("Exile<1+/Artifact+withTotalPowerEQX>"); len(c.Unknown) == 0 || len(c.Exile) != 0 {
		t.Errorf("unknown group predicate parsed to %+v", c)
	}
}

// TestParseChooseCreatureTypeCost: ChooseCreatureType<1> is a modelled part
// (no phantom generic), any other count stays reported.
func TestParseChooseCreatureTypeCost(t *testing.T) {
	c := ParseCost("2 G ChooseCreatureType<1>")
	if c.ChooseType != 1 || c.Generic != 2 || len(c.Unknown) != 0 {
		t.Fatalf("parsed %+v", c)
	}
	if got := FormatCost(ParseCost("ChooseCreatureType<1> Behold<2/Creature.ChosenType>")); got != "ChooseCreatureType<1> Behold<2/Creature.ChosenType>" {
		t.Errorf("FormatCost = %q", got)
	}
	if c := ParseCost("ChooseCreatureType<2>"); len(c.Unknown) == 0 {
		t.Errorf("a two-type choice parsed as modelled: %+v", c)
	}
}

func TestManaSymbolsOfColor(t *testing.T) {
	for cost, want := range map[string]int32{
		"B B B B": 4, "3 B B": 2, "B/G B/P 2/B": 3, "B/G/P": 1, "X U U": 0,
	} {
		if got := ManaSymbolsOfColor(ParseCost(cost), 'B'); got != want {
			t.Errorf("%s: %d black symbols, want %d", cost, got, want)
		}
	}
}

func TestSpecVarCompares(t *testing.T) {
	for spec, want := range map[string]string{
		"Creature.Other+cmcEQX":              "Creature.Other",
		"Artifact.cmcGTX":                    "Artifact",
		"Creature.cmcEQX,Artifact.cmcEQX":    "Creature,Artifact",
		"Card.powerGE2":                      "Card.powerGE2",
		"Creature.YouCtrl+cmcLEX+Other":      "Creature.YouCtrl+Other",
		"Creature.namedXanthic Statue+Other": "Creature.namedXanthic Statue+Other",
	} {
		if got := StripVarCompares(spec, "X"); got != want {
			t.Errorf("StripVarCompares(%q) = %q, want %q", spec, got, want)
		}
	}
	if !SpecHasTerm("Creature.ChosenType", "ChosenType") || SpecHasTerm("Creature.IsNotChosenType", "ChosenType") {
		t.Error("SpecHasTerm misreads ChosenType")
	}
	if got := ReplaceTerm("Creature.ChosenType", "ChosenType", "Elf"); got != "Creature.Elf" {
		t.Errorf("ReplaceTerm = %q", got)
	}
	if got := ReplaceTerm("Creature.ChosenType", "ChosenType", ""); got != "Creature" {
		t.Errorf("ReplaceTerm drop = %q", got)
	}
}
