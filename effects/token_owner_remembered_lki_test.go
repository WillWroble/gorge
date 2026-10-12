package effects

import (
	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/internal/testutil"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/events"
	"github.com/adams-shaun/gorge/state"
)

// TestTokenOwnerRememberedControllerReadsLastKnownController pins CR 608.2h
// for Unyielding Gatekeeper's shape: a permanent seat 0 owns but seat 1
// controls is exiled with RememberLKI$ True, and the chained Token's
// TokenOwner$ RememberedController is the controller it had on the
// battlefield (seat 1), not the owner it reverted to in exile.
func TestTokenOwnerRememberedControllerReadsLastKnownController(t *testing.T) {
	h := newHost(t, 2)
	h.g.Tokens = tokenFixtures(t)
	victim := h.g.AddObject(mkCard(t, "Name:Fixture Relic\nTypes:Artifact\nOracle:x\n"), 0)
	victim.Zone = state.ZBattlefield
	victim.Controller = 1
	h.g.SetZone(state.ZBattlefield, 0, append(h.g.Zone(state.ZBattlefield, 0), victim.ID))
	if o := h.g.Obj(victim.ID); o == nil || o.Zone != state.ZBattlefield || o.Controller != 1 || o.Owner != 0 {
		t.Fatalf("precondition: victim = %+v, want seat 0 owned, seat 1 controlled, on battlefield", o)
	}

	src := "Name:Fixture Gatekeeper\nManaCost:1 W\nTypes:Creature Cleric\nPT:3/2\n" +
		"A:SP$ ChangeZone | ValidTgts$ Permanent | Origin$ Battlefield | Destination$ Exile | RememberLKI$ True | SubAbility$ DBToken\n" +
		"SVar:DBToken:DB$ Token | TokenScript$ r_1_1_goblin | TokenOwner$ RememberedController\n" +
		"Oracle:x\n"
	card := mkCard(t, src)
	c := &Ctx{Source: victim.ID, Controller: 0, Targets: []state.Target{{Obj: victim.ID}}}
	Resolve(h, c, card.Faces[0].Abilities[0])

	o := h.g.Obj(victim.ID)
	if o == nil || o.Zone != state.ZExile || o.Controller != 0 {
		t.Fatalf("victim = %+v, want exiled with live controller reset to owner 0", o)
	}
	if bf := h.g.Zone(state.ZBattlefield, 1); len(bf) != 1 {
		t.Fatalf("last-known controller's battlefield = %v, want 1 token", bf)
	}
	for _, id := range h.g.Zone(state.ZBattlefield, 0) {
		if tok := h.g.Obj(id); tok != nil && tok.IsToken {
			t.Fatalf("owner's battlefield holds a token %+v; token must go to the last-known controller", tok)
		}
	}
	for _, ev := range h.log {
		if ev.Kind == events.Note && strings.Contains(ev.Text, "unimplemented") {
			t.Fatalf("handler not registered: %q", ev.Text)
		}
	}
}

// TestTokenOwnerRememberedControllerCarrierCensus is the corpus ratchet for
// the mechanism: every Token line whose TokenOwner$ is RememberedController
// reads a remembered object's controller AFTER the chain may have moved it.
// The carrier set is pinned; a new carrier must be reviewed for whether its
// chain moves the remembered card (and so needs the RememberLKI$ capture
// rememberedControllersLKI reads) before it is added here.
func TestTokenOwnerRememberedControllerCarrierCensus(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	known := map[string]bool{"Unyielding Gatekeeper": true, "The Curse of Fenric": true, "Terastodon": true}
	found := map[string]bool{}
	for _, card := range reg.AllCards() {
		for _, f := range card.Faces {
			for _, body := range f.SVars {
				if strings.Contains(body, "DB$ Token") && strings.Contains(body, "TokenOwner$ RememberedController") {
					found[f.Name] = true
				}
			}
			for _, sa := range f.Abilities {
				if sa != nil && sa.API == "Token" && strings.HasPrefix(strings.TrimSpace(sa.ParamStr(cards.PKTokenOwner)), "RememberedController") {
					found[f.Name] = true
				}
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("census found no carriers: the scan is not reading the corpus")
	}
	for name := range found {
		if !known[name] {
			t.Errorf("new TokenOwner$ RememberedController carrier %q: review whether its chain moves the remembered card", name)
		}
	}
	for name := range known {
		if !found[name] {
			t.Errorf("pinned carrier %q no longer found", name)
		}
	}
	// Unyielding Gatekeeper is the one carrier whose first step exiles the
	// remembered card, so its chain must keep the RememberLKI$ capture the
	// last-known-controller read depends on.
	gk, ok := reg.Lookup("Unyielding Gatekeeper")
	if !ok || !strings.Contains(gk.Faces[0].SVars["TrigExile"], "RememberLKI$ True") {
		t.Fatalf("Unyielding Gatekeeper no longer remembers LKI: %+v", gk)
	}
}
