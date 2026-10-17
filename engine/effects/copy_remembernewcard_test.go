package effects

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// rememberNewCardCarriers is the corpus-wide ratchet: every card whose
// CopySpellAbility script carries RememberNewCard$ True. A new carrier fails
// TestRememberNewCardCensus until it is added here, which is the prompt to
// check its follow-up reads the copy.
var rememberNewCardCarriers = []string{
	"Choreographed Sparks",
	"Gandalf, Westward Voyager",
	"Mizzix, Replica Rider",
	"Nalfeshnee",
	"Spider-Verse",
	"Spinerock Tyrant",
}

// TestRememberNewCardCensus walks the whole corpus for CopySpellAbility SVars
// declaring RememberNewCard$ True and pins the carrier set.
func TestRememberNewCardCensus(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	got := map[string]bool{}
	for _, c := range reg.AllCards() {
		for _, f := range c.Faces {
			for _, body := range f.SVars {
				if strings.Contains(body, "CopySpellAbility") && strings.Contains(body, "RememberNewCard$ True") {
					got[c.Faces[0].Name] = true
				}
			}
		}
	}
	var names []string
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, rememberNewCardCarriers) {
		t.Fatalf("RememberNewCard$ CopySpellAbility carriers = %v, want %v", names, rememberNewCardCarriers)
	}
}

// TestCopySpellAbilityRememberNewCardRemembersCopy drives Spinerock Tyrant's
// real compiled copy SA and pins that the minted copy lands in the chain's
// remembered set (so DBPump's Defined$ Remembered finds it) and on the
// source's event-backed list.
func TestCopySpellAbilityRememberNewCardRemembersCopy(t *testing.T) {
	real := corpusCopySA(t, "Spinerock Tyrant")
	if !strings.EqualFold(strings.TrimSpace(real.Params["RememberNewCard"]), "True") {
		t.Fatalf("precondition: Spinerock copy SA lacks RememberNewCard$ True: %+v", real.Params)
	}
	h := newHost(t, 2)
	spell := spellOnStack(t, h, "A:SP$ DealDamage | ValidTgts$ Any | NumDmg$ 3", 0)
	if spell.Zone != state.ZStack {
		t.Fatalf("precondition: spell zone=%s, want the stack", spell.Zone)
	}
	cp := *real
	cp.Sub = nil // the cleanup would erase ctx.Remembered by design
	ctx := &Ctx{Source: spell.ID, Controller: 0, Remembered: []state.Target{{Obj: spell.ID}}}
	Resolve(h, ctx, &cp)
	var copyID state.ObjID
	for _, ob := range h.g.Objs {
		if ob.IsCopy {
			copyID = ob.ID
		}
	}
	if copyID == 0 || copyID == spell.ID {
		t.Fatalf("precondition: no copy minted (copy=%d spell=%d)", copyID, spell.ID)
	}
	if len(ctx.Remembered) != 2 || ctx.Remembered[1].Obj != copyID {
		t.Fatalf("Remembered=%+v, want [spell %d, copy %d]", ctx.Remembered, spell.ID, copyID)
	}
}

// TestSpinerockPumpChainReadsCopy resolves the whole real chain (copy ->
// DBPump Defined$ TriggeredCard & Remembered, PumpZone$ Stack -> cleanup):
// the follow-up Pump must register the Wither grant on the minted copy.
func TestSpinerockPumpChainReadsCopy(t *testing.T) {
	real := corpusCopySA(t, "Spinerock Tyrant")
	if real.Sub == nil || real.Sub.API != "Pump" {
		t.Fatalf("precondition: copy SA's sub is not the Pump: %+v", real.Sub)
	}
	h := newHost(t, 2)
	spell := spellOnStack(t, h, "A:SP$ DealDamage | ValidTgts$ Any | NumDmg$ 3", 0)
	ctx := &Ctx{Source: spell.ID, Controller: 0, Remembered: []state.Target{{Obj: spell.ID}}}
	Resolve(h, ctx, real)
	var copyID state.ObjID
	for _, ob := range h.g.Objs {
		if ob.IsCopy {
			copyID = ob.ID
		}
	}
	if copyID == 0 {
		t.Fatalf("precondition: no copy minted")
	}
	for _, ce := range h.continuous {
		if ce.Source == copyID && slices.Contains(ce.AddKeywords, "Wither") {
			return
		}
	}
	t.Fatalf("no Wither grant registered on copy %d: %+v", copyID, h.continuous)
}
