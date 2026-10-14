package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/rules/trigmatch"
)

// This file is the census behind AGENTS.md's Known-approximations row for the
// Effect-delivered StaticAbilities$/Triggers$ registration (effects/effect.go).
// It does NOT keep a hand list of supported modes: the registered static modes
// are read straight out of the switch in effects/effect.go (its `case
// cards.Static...` labels) plus the one mode effectUnsupportedStatic admits
// (IgnoreHexproof), and the registered trigger modes are read from the live
// trigger registry (trigmatch.Lookup), which is the same table
// Engine.TriggerModeSupported consults. The corpus walk then reports every
// mode reached through an Effect in the pinned corpus and whether the code
// registers it or emits the unimplemented Note.

var staticCaseRE = regexp.MustCompile(`cards\.Static([A-Z][A-Za-z]+)`)

// effectRegisteredStaticModes parses the Effect switch's own case labels.
func effectRegisteredStaticModes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "effects", "effect.go"))
	if err != nil {
		t.Fatalf("read effects/effect.go: %v", err)
	}
	modes := map[string]bool{}
	for _, m := range staticCaseRE.FindAllStringSubmatch(string(src), -1) {
		if m[1] == "ModeOf" { // cards.StaticModeOf, not a mode label
			continue
		}
		modes[m[1]] = true
	}
	// IgnoreHexproof is not a switch label: effectUnsupportedStatic registers
	// it from the default arm. Detect that read rather than assume it.
	ih, err := os.ReadFile(filepath.Join("..", "effects", "effect_ignorehexproof.go"))
	if err != nil {
		t.Fatalf("read effects/effect_ignorehexproof.go: %v", err)
	}
	if strings.Contains(string(ih), "mode == ModeIgnoreHexproof") {
		modes["IgnoreHexproof"] = true
	}
	return modes
}

// effectCorpusModes returns, per mode, the set of corpus files whose Effect
// body (an SVar referenced from a StaticAbilities$ or Triggers$ field) names
// that mode.
func effectCorpusModes(t *testing.T) (static, trigger map[string]map[string]bool) {
	t.Helper()
	root := filepath.Join(testutil.CorpusDirOrSkip(t), "cardsfolder")
	static = map[string]map[string]bool{}
	trigger = map[string]map[string]bool{}
	saRE := regexp.MustCompile(`StaticAbilities\$([^|\n]*)`)
	trRE := regexp.MustCompile(`Triggers\$([^|\n]*)`)
	bodyRE := regexp.MustCompile(`(?m)^SVar:([^:]+):Mode\$([^\n|]*)`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".txt") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		txt := string(b)
		saRefs, trRefs := map[string]bool{}, map[string]bool{}
		for _, m := range saRE.FindAllStringSubmatch(txt, -1) {
			for _, tok := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
				saRefs[tok] = true
			}
		}
		for _, m := range trRE.FindAllStringSubmatch(txt, -1) {
			for _, tok := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
				trRefs[tok] = true
			}
		}
		for _, m := range bodyRE.FindAllStringSubmatch(txt, -1) {
			name := strings.TrimSpace(m[1])
			for _, mode := range strings.FieldsFunc(m[2], func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
				if saRefs[name] {
					if static[mode] == nil {
						static[mode] = map[string]bool{}
					}
					static[mode][path] = true
				}
				if trRefs[name] {
					if trigger[mode] == nil {
						trigger[mode] = map[string]bool{}
					}
					trigger[mode][path] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus: %v", err)
	}
	return static, trigger
}

// TestEffectStaticCensus is a report: it logs every mode reached through an
// Effect in the corpus and whether the code registers it, and pins the modes
// that still Note so a mode silently gaining or losing its registration fails
// here and the AGENTS.md row cannot drift from the code.
func TestEffectStaticCensus(t *testing.T) {
	staticHandled := effectRegisteredStaticModes(t)
	staticCorpus, triggerCorpus := effectCorpusModes(t)

	names := func(m map[string]map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	var staticNotes, triggerNotes []string
	var b strings.Builder
	fmt.Fprintf(&b, "Effect static modes reached in the corpus (%d):\n", len(staticCorpus))
	for _, mode := range names(staticCorpus) {
		state := "REGISTERED"
		if !staticHandled[mode] {
			state = "NOTES"
			staticNotes = append(staticNotes, mode)
		}
		fmt.Fprintf(&b, "  %-4s %-28s %d files\n", state, mode, len(staticCorpus[mode]))
	}
	fmt.Fprintf(&b, "Effect trigger modes reached in the corpus (%d):\n", len(triggerCorpus))
	for _, mode := range names(triggerCorpus) {
		state := "REGISTERED"
		if trigmatch.Lookup(mode) == nil && mode != "ChangesController" {
			state = "NOTES"
			triggerNotes = append(triggerNotes, mode)
		}
		fmt.Fprintf(&b, "  %-4s %-28s %d files\n", state, mode, len(triggerCorpus[mode]))
	}
	t.Logf("\n%s", b.String())

	gotStatic := strings.Join(staticNotes, ",")
	if gotStatic != effectStaticStillNotes {
		t.Errorf("Effect static modes that still Note changed:\n got %s\nwant %s", gotStatic, effectStaticStillNotes)
	}
	gotTrigger := strings.Join(triggerNotes, ",")
	if gotTrigger != effectTriggerStillNotes {
		t.Errorf("Effect trigger modes that still Note changed:\n got %s\nwant %s", gotTrigger, effectTriggerStillNotes)
	}
}

// effectStaticStillNotes and effectTriggerStillNotes pin the census above.
// They are the modes AGENTS.md's Effect row names as still unimplemented.
const (
	effectStaticStillNotes = "AssignCombatDamageAsUnblocked,AssignNoCombatDamage,AttackVigilance," +
		"BlockRestrict,CanAdapt,CantBeActivated,CantBeCast,CantBecomeMonarch,CantBlock," +
		"CantChangeLife,CantLoseLife,CantPhaseIn,CantPhaseOut,IgnoreLegendRule,IgnoreShroud," +
		"MinMaxBlocker,UntapOtherPlayer"
	effectTriggerStillNotes = "Abandoned,Adapt,AttackerBlocked,AttackerBlockedByCreature," +
		"AttackerUnblocked,Blocks,Shuffled,VisitAttraction"
)
