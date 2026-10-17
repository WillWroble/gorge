package rules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// copyInheritedScenario is the committed Mirrorpool scenario
// "copy-target-spell-deals-damage-twice" inline: a Lightning Bolt on the stack
// is copied by Mirrorpool's MayChooseTarget$ ability, so the copy's CR 707.10c
// new-target election carries exactly one inherited target (the Bolt's seat 1).
const copyInheritedScenario = `{
 "name": "copy-inherited-count",
 "cr": ["707.10a", "707.10c"],
 "why": "Mirrorpool copies a Lightning Bolt on the stack; the copy's new-target election inherits the Bolt's one target.",
 "setup": {
  "p0": {"battlefield": ["Mirrorpool"], "hand": ["Lightning Bolt"]}
 },
 "steps": [
  {"op": "cast", "seat": 0, "card": "p0:Lightning Bolt", "mana": "R", "targets": ["p1"]},
  {"op": "activate", "seat": 0, "card": "p0:Mirrorpool", "mana": "CCC", "ability": "copy target instant or sorcery", "targets": ["p0:Lightning Bolt"]},
  {"op": "resolve"}
 ],
 "expect": [{"life": {"p0": 20, "p1": 14}}]
}`

// TestOracleTranscriptRecordsCopyInheritedCount runs a real corpus copy (the
// Mirrorpool MayChooseTarget$ ability copying a Lightning Bolt) through the
// oracle runner and asserts the transcript records the copy ask's inherited
// target count. XMage poses one "Change this N of M target?" chooseUse per
// inherited target, so the generator cannot script the copy ask without it.
func TestOracleTranscriptRecordsCopyInheritedCount(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	res, err := RunOracleScenarioJSON(reg, []byte(copyInheritedScenario))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v\n%s", res.Fails, strings.Join(res.Transcript, "\n"))
	}
	found := false
	for _, d := range res.Decisions {
		if d.Resume != "copy_targets" {
			continue
		}
		// PRECONDITION: the ask really is the copy's target election, and the
		// copy really inherited one target; a zero count below would otherwise
		// pass by accident on a non-copy decision.
		if d.Kind != "target" {
			t.Fatalf("copy_targets decision is not a target ask: %+v", d)
		}
		if d.CopyInherited != 1 {
			t.Fatalf("copy ask recorded CopyInherited=%d, want 1: %+v", d.CopyInherited, d)
		}
		found = true
	}
	if !found {
		t.Fatal("scenario posed no copy_targets decision; the count assertion proved nothing")
	}
}

// TestOracleDecisionCopyInheritedKeyIsOmitEmpty pins that the new field never
// changes a non-copy row's JSON: only a copy ask carries "copy_inherited".
func TestOracleDecisionCopyInheritedKeyIsOmitEmpty(t *testing.T) {
	plain, err := json.Marshal(OracleDecision{Step: 0, Kind: "target", Min: 1, Max: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "copy_inherited") {
		t.Fatalf("non-copy decision serialised copy_inherited: %s", plain)
	}
	copyDec, err := json.Marshal(OracleDecision{Step: 0, Kind: "target", Resume: "copy_targets", CopyInherited: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(copyDec), `"copy_inherited":2`) {
		t.Fatalf("copy decision dropped copy_inherited: %s", copyDec)
	}
}
