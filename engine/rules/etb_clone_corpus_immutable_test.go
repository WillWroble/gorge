package rules

import (
	"slices"
	"sort"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/state"
)

// TestFleshDuplicateRiderLeavesCorpusFaceImmutable pins the fold's
// isolation from the shared corpus. For a plain battlefield creature the
// Face the clone fold reads (engine/state/object.go returns o.Card.Faces[i]) IS
// the process-global corpus face pointer, so the ifnew expansion in
// events.foldClonePermanent must run on deep-copied Keywords/Triggers/
// Repls/SVars, never on the aliased backing fields -- otherwise the
// expansion setSVar'd __kwVanishing* into the shared map, contaminating
// every concurrent match on cards.SharedCorpus. Kalitas, Bloodchief of
// Ghet carries non-nil SVars in its printed face: exactly the shape where
// the shallow copy leaked (the template faces the other tests use --
// Colossal Dreadmaw, Deep Forest Hermit -- have nil SVars and could not
// fail on the leak).
func TestFleshDuplicateRiderLeavesCorpusFaceImmutable(t *testing.T) {
	t.Parallel()
	e, id, tplID := fleshDuplicateEntry(t, "Kalitas, Bloodchief of Ghet")
	tpl := e.G.Obj(tplID)
	if tpl == nil || tpl.Zone != state.ZBattlefield {
		t.Fatalf("template = %+v, want on the battlefield", tpl)
	}
	tplFace := tpl.Face()
	// Precondition 1: the template's face carries SVars -- without them
	// the leak this test pins cannot fire and the run is vacuous.
	if len(tplFace.SVars) == 0 {
		t.Fatal("Kalitas, Bloodchief of Ghet's face has no SVars; pick another SVar-bearing template")
	}
	// Precondition 2: the template has no vanishing, so the conditional
	// actually fires and the expansion runs at all.
	for _, kw := range tplFace.Keywords {
		if cards.KeywordHead(kw) == "Vanishing" {
			t.Fatalf("template already has vanishing (%q); the rider would stay silent", kw)
		}
	}
	before := svarSnapshot(tplFace.SVars)
	idx := etbElectionOption(t, e.Pending(), tplID)
	submitChoices(t, e, idx)
	o := e.G.Obj(id)
	if o == nil || o.Zone != state.ZBattlefield || o.Face().Name != "Kalitas, Bloodchief of Ghet" {
		t.Fatalf("entry = %+v, want a Kalitas, Bloodchief of Ghet copy", o)
	}
	if got := o.Counter("TIME"); got != 3 {
		t.Fatalf("granted vanishing entered with %d TIME counters, want 3", got)
	}
	if after := svarSnapshot(tplFace.SVars); !slices.Equal(before, after) {
		t.Fatalf("corpus face SVar map changed %v -> %v; the ifnew fold leaked into the shared corpus face", before, after)
	}
	// The minted expansion belongs on the copy's synthetic face alone.
	rider := false
	for _, kw := range o.Face().Keywords {
		if cards.KeywordHead(kw) == "Vanishing" {
			rider = true
		}
	}
	if !rider {
		t.Fatalf("copy face keywords %v, want the granted Vanishing:3", o.Face().Keywords)
	}
}

func svarSnapshot(svars map[string]string) []string {
	out := make([]string, 0, len(svars))
	for k, v := range svars {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
