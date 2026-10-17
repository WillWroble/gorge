package rules

// The oracle runner's contract check. The scenario corpora (the hand-authored
// Oracle-text audit and the XMage ports) and their drivers live in
// gorge-compliance (github.com/adams-shaun/gorge-compliance), which plays
// them through the exported API in oracle_export.go and
// oracle_audit_export.go. gorge keeps only the scenario files its own card
// tests load (testdata/oracle/<family>/<card>.json: Wizard's Staff, Heirloom
// Epic, Gray Merchant of Asphodel, Rhovanion Rampager, Supper for Spiders,
// Uncover the Moon-Letters, and the census carriers Witness Protection,
// Purphoros and Bard's Company); gorge-compliance's audit reads them together
// with its own, so a scenario file lives in exactly one repository.
//
// TestOracleRunnerContract plays those residual files through the EXPORTED
// entry points, so a runner change that breaks the API gorge-compliance
// depends on fails here, in gorge, and not first in the other repository.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// loadOracleFiles reads every residual testdata/oracle/<family>/*.json file,
// keyed by its path.
func loadOracleFiles(t *testing.T) map[string]oracleFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "oracle", "*", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no oracle scenario files: %v", err)
	}
	out := map[string]oracleFile{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var f oracleFile
		if err := dec.Decode(&f); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if f.Card == "" || f.Family == "" || len(f.Scenarios) == 0 {
			t.Fatalf("%s: card, family and at least one scenario are required", p)
		}
		if fam := filepath.Base(filepath.Dir(p)); fam != f.Family {
			t.Fatalf("%s: family %q does not match its directory %q", p, f.Family, fam)
		}
		out[p] = f
	}
	return out
}

// residualScenarios is every residual scenario as the raw JSON object an
// outside driver hands the exported API.
func residualScenarios(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "oracle", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]json.RawMessage{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Card      string            `json:"card"`
			Scenarios []json.RawMessage `json:"scenarios"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		for _, sc := range f.Scenarios {
			var h struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(sc, &h); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			out[f.Card+"/"+h.Name] = sc
		}
	}
	return out
}

// TestOracleScenarioShapeCheck: the corpus-free shape check accepts every
// residual scenario and names each kind of authoring error it exists for.
func TestOracleScenarioShapeCheck(t *testing.T) {
	t.Parallel()
	loadOracleFiles(t) // the file level, strictly
	for key, raw := range residualScenarios(t) {
		if probs := CheckOracleScenarioJSON(raw); len(probs) > 0 {
			t.Errorf("%s: %v", key, probs)
		}
	}
	for _, c := range []struct{ raw, want string }{
		{`{"name":"x","bogus":1}`, "unknown field"},
		{`{"name":"x","steps":[{"op":"teleport","seat":0}]}`, `unknown op "teleport"`},
		{`{"name":"x","steps":[{"op":"cast","seat":0,"card":"Forest"}]}`, "bad card ref"},
		{`{"name":"x","expect":[{"card":"p0:Forest#0"}]}`, "bad ordinal"},
	} {
		probs := CheckOracleScenarioJSON([]byte(c.raw))
		if !strings.Contains(strings.Join(probs, "\n"), c.want) {
			t.Errorf("%s: problems %q, want one naming %q", c.raw, probs, c.want)
		}
	}
}

// TestOracleRunnerContract plays every residual scenario through both
// exported entry points: the audit mode passes and replays from its log
// alone, and the compliance pipeline's fixture-mode entry point decodes and
// runs it without a harness error.
func TestOracleRunnerContract(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	scenarios := residualScenarios(t)
	if len(scenarios) < 9 {
		t.Fatalf("%d residual scenarios: the contract check needs the files gorge's card tests load", len(scenarios))
	}
	for key, raw := range scenarios {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			res, err := RunOracleAuditScenarioJSON(reg, raw, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Fails) > 0 {
				t.Errorf("audit mode:\n  FAIL: %s\n  transcript:\n    %s", strings.Join(res.Fails, "\n  FAIL: "), strings.Join(res.Transcript, "\n    "))
			}
			if res.ReplayDiff != "" {
				t.Errorf("log-only replay differs:\n%s", res.ReplayDiff)
			}
			fx, err := RunOracleScenarioJSON(reg, raw)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range fx.Fails {
				if strings.HasPrefix(f, "harness: panic: ") {
					t.Errorf("fixture mode panicked: %s", f)
				}
			}
		})
	}
	if _, err := RunOracleAuditScenarioJSON(reg, []byte(`{"name":"x","bogus":1}`), false); err == nil {
		t.Error("an unknown field decoded without error")
	}
}
