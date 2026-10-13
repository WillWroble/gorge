package templates

import (
	"path/filepath"
	"testing"

	"github.com/adams-shaun/gorge/compliance"
	"github.com/adams-shaun/gorge/compliance/levelb"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/rules"
)

// lb-xpending ratchet. XMage poses a trigger-order ask (CR 603.3b) and a
// "you may pay" triggered cost (CR 603.5) inside the pass that put the
// trigger on the stack, so its checkpoint already shows the answer. Gorge
// answered them one step later (on the following resolve/pass_to step),
// leaving the checkpoint with an empty stack or an unpaid cost. The runner now
// settles them before the checkpoint (rules.settleTriggerAsks); this census
// counts, over every generated ECL/TLA/EOE trigger and static scenario, the
// checkpoints still taken with one pending (OracleResult.PendingTriggerAsks).
//
// The pass-posed count is the precondition: it proves the walk reaches the
// shape, so a zero late count cannot be the vacuous result of a census that
// never meets a trigger ask.
func TestTriggerAsksAreSettledBeforeTheCheckpoint(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	root := filepath.Join("..", "..", "..")
	has := func(n string) bool { _, ok := reg.Lookup(n); return ok }
	folded := compliance.FoldedNames(reg)

	seen := map[string]bool{}
	var names []string
	for _, set := range []string{"ECL", "TLA", "EOE"} {
		printed, err := compliance.LoadPrinted(filepath.Join(root, "compliance", "printed"), set)
		if err != nil {
			t.Fatalf("%s: %v", set, err)
		}
		for _, n := range printed.Cards {
			if card, ok := compliance.CorpusNameFold(has, folded, n); ok && !seen[card] {
				seen[card] = true
				names = append(names, card)
			}
		}
	}
	posedAtPass, pending, scenarios := 0, 0, 0
	for _, name := range names {
		c, _ := reg.Lookup(name)
		for _, r := range levelb.Requirements(c) {
			if r.Family != "trigger" && r.Family != "static" {
				continue
			}
			it, skip := GenerateB(reg, name, r)
			if skip != nil {
				continue
			}
			res, err := rules.RunOracleScenarioJSON(reg, it.Raw())
			if err != nil || len(res.Fails) != 0 {
				continue
			}
			scenarios++
			if res.PendingTriggerAsks != 0 {
				pending += res.PendingTriggerAsks
				t.Errorf("%s %s: %d checkpoint(s) taken with a trigger ask pending", name, r.Key, res.PendingTriggerAsks)
			}
			for _, d := range res.Decisions {
				isTriggerAsk := d.GorgeKind == "trigger_order"
				for _, k := range d.PickKinds {
					isTriggerAsk = isTriggerAsk || k == "trigger_cost_pay" || k == "trigger_cost_decline"
				}
				if isTriggerAsk && d.Step >= 1 && d.Step < len(it.Steps) && it.Steps[d.Step].Op == "pass" {
					posedAtPass++
				}
			}
		}
	}
	if scenarios == 0 {
		t.Fatal("census ran no scenario")
	}
	if posedAtPass == 0 {
		t.Fatal("precondition: no trigger ask was settled on its pass step; the census never meets the shape")
	}
	t.Logf("scenarios=%d settled-at-pass=%d pending=%d", scenarios, posedAtPass, pending)
}

// Famished Worldsire's Ward is its trigger#0.1; the card is a printed 0/0 that
// dies as it enters, so nothing can target it. The skip names that cause
// instead of the bare "did not fire".
func TestZeroToughnessWardSkipIsNamed(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	c, ok := reg.Lookup("Famished Worldsire")
	if !ok {
		t.Fatal("precondition: Famished Worldsire is not in the corpus")
	}
	found := false
	for _, r := range levelb.Requirements(c) {
		if r.Key != "trigger#0.1" {
			continue
		}
		found = true
		if r.Sub != "trigger.becomes-target" {
			t.Fatalf("precondition: trigger#0.1 is %q, want trigger.becomes-target", r.Sub)
		}
		_, skip := GenerateB(reg, "Famished Worldsire", r)
		if skip == nil {
			t.Fatal("trigger#0.1 is now served; the zero-toughness skip is dead")
		}
		const want = "trigger creature enters with toughness 0 and dies before it can be targeted"
		if skip.Reason != want {
			t.Fatalf("skip = %q, want %q", skip.Reason, want)
		}
	}
	if !found {
		t.Fatal("precondition: no trigger#0.1 requirement")
	}
}
