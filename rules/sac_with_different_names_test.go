package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// Transmutation Font's "{3}, {T}, Sacrifice three artifact tokens with
// different names" (Forge's Sac<3/Artifact.token+WithDifferentNames>): the
// modifier constrains the sacrificed SET (CR 601.2h: the payer pays the cost
// as stated), so three artifact tokens of three names pay it and three tokens
// sharing a name do not. Before the modifier was read the per-object filter
// failed closed on it and the ability was never offered.
func TestSacWithDifferentNames(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	run := func(makers []string) []string {
		t.Helper()
		hand := `"` + strings.Join(makers, `","`) + `"`
		var steps []string
		seen := map[string]int{}
		for _, m := range makers {
			seen[m]++
			ref := "p0:" + m
			if seen[m] > 1 {
				ref += "#2"
			}
			mana := "G"
			if m == "Thraben Inspector" {
				mana = "W"
			}
			steps = append(steps, `{"op":"cast","seat":0,"card":"`+ref+`","mana":"`+mana+`"},{"op":"resolve"},{"op":"resolve"}`)
		}
		steps = append(steps, `{"op":"activate","seat":0,"card":"p0:Transmutation Font","mana":"CCC","ability_index":1},{"op":"resolve"}`)
		sc := `{"name":"font","cr":["601.2h"],"why":"inline","setup":{"p0":{"battlefield":["Transmutation Font"],"hand":[` + hand + `]}},"steps":[` +
			strings.Join(steps, ",") + `]}`
		res, err := RunOracleScenarioJSON(reg, []byte(sc))
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return res.Fails
	}
	// Two Food tokens and a Clue: only two names, so not payable.
	if fails := run([]string{"Gilded Goose", "Gilded Goose", "Thraben Inspector"}); len(fails) == 0 || !strings.Contains(strings.Join(fails, " "), "activate p0:Transmutation Font not offered") {
		t.Fatalf("Food, Food, Clue paid a with-different-names cost: fails=%v", fails)
	}
}

// TestSacWithDifferentNamesPays: one token each of three names pays it.
func TestSacWithDifferentNamesPays(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	sc := `{"name":"font-pay","cr":["601.2h"],"why":"inline","setup":{"p0":{"battlefield":["Transmutation Font"],"hand":["Gilded Goose","Thraben Inspector","Strike It Rich"]}},"steps":[` +
		`{"op":"cast","seat":0,"card":"p0:Gilded Goose","mana":"G"},{"op":"resolve"},{"op":"resolve"},` +
		`{"op":"cast","seat":0,"card":"p0:Thraben Inspector","mana":"W"},{"op":"resolve"},{"op":"resolve"},` +
		`{"op":"cast","seat":0,"card":"p0:Strike It Rich","mana":"R"},{"op":"resolve"},` +
		`{"op":"activate","seat":0,"card":"p0:Transmutation Font","mana":"CCC","ability_index":1},{"op":"resolve"}]}`
	res, err := RunOracleScenarioJSON(reg, []byte(sc))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("Food, Clue, Treasure did not pay the with-different-names cost: %v", res.Fails)
	}
	last := res.Snapshots[len(res.Snapshots)-1]
	for _, p := range last.Permanents {
		if strings.Contains(p.Ref, ":token:") {
			t.Fatalf("a token survived the sacrifice: %+v", p)
		}
	}
}
