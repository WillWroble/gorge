package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestOracleLegendKeepSurvivorFallback pins the oracle runner's fallback for a
// CR 704.5j keep whose battlefield-order first member is itself a casualty of
// the same state-based-action batch (CR 704.3, CR 704.5f). Grub, Notorious
// Auntie attacks and blights himself (a -1/-1 counter makes him 1/0) to make a
// tapped and attacking token copy of himself, which enters 2/1: the legend
// rule and the toughness action are one batch. Keeping option 0 (Grub) lost
// both objects; the fallback keeps the surviving token, which is also XMage's
// outcome (Grub, Storied Matriarch trigger#1.0 / combat#1.attack diverged on
// exactly this).
func TestOracleLegendKeepSurvivorFallback(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	const sc = `{"name":"grub-keep","setup":{"p0":{"battlefield":["Grub, Storied Matriarch"],"back_face":["Grub, Storied Matriarch"]},"p1":{"battlefield":["Grizzly Bears"]}},` +
		`"steps":[{"op":"attack","seat":0,"attackers":["p0:Grub, Storied Matriarch"],"defender":"p1","answers":[{"kind":"choose","pick":["Do not pay"]}]},` +
		`{"op":"resolve","seat":0,"answers":[{"kind":"choose","pick":["Blight 1"]}]}]}`
	res, err := RunOracleScenarioJSON(reg, []byte(sc))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fails) != 0 {
		t.Fatalf("scenario failed: %v", res.Fails)
	}
	// Precondition: the keep was really posed between same-named members and
	// answered by the fallback, so the assertion below is about the fallback.
	keep := -1
	for i, d := range res.Decisions {
		if len(d.PickKinds) == 1 && d.PickKinds[0] == "keep" {
			keep = i
		}
	}
	if keep < 0 {
		t.Fatalf("no legend-rule keep was posed; decisions %+v", res.Decisions)
	}
	d := res.Decisions[keep]
	if d.Options != 2 || d.Picks[0] != d.First {
		t.Fatalf("keep %+v: want two same-named members", d)
	}
	if d.PickIdx[0] != 1 {
		t.Fatalf("fallback kept option %d (%v), want option 1, the member that survives the batch", d.PickIdx[0], d.ObjectPicks)
	}
	last := res.Snapshots[len(res.Snapshots)-1]
	var survivors []OracleSnapPerm
	for _, p := range last.Permanents {
		if p.Controller == 0 && p.Name == "Grub, Notorious Auntie" {
			survivors = append(survivors, p)
		}
	}
	if len(survivors) != 1 || !survivors[0].Token || survivors[0].PT != "2/1" {
		t.Fatalf("p0's Grubs after the batch = %+v, want only the 2/1 token", survivors)
	}
}
