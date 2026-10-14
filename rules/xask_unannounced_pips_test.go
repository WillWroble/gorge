package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// TestHybridXAbilityOffersOnlyAnnounceableX is the regression for the
// 20000-game census finding "activation aborted: cost no longer payable"
// (34 games, all at Fiend Artisan): xAsk priced the candidate announcement
// against the X-folded cost with the UNANNOUNCED pips dropped, so pool
// {G}{G} offered X=2 for the `{X}{B/G}` cost -- {X} alone prices at 2, but
// the pip needs a third unit no pip-face assignment can pay. Every pip face
// then came up infeasible and the mana ask's defensive arm posted a doomed
// payment, which the engine reversed at payCast (CR 733.1). CR 601.2b: the
// announcement must be one the payment can settle -- the X price has to
// carry the still-unannounced pips.
func TestHybridXAbilityOffersOnlyAnnounceableX(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	artisan, ok := reg.Lookup("Fiend Artisan")
	if !ok {
		t.Fatal("Fiend Artisan not found in the compiled corpus registry")
	}
	e := handEngine(t, artisan)
	// The {T} and Sac<1/Creature.Other> parts need the walker and another
	// creature; neither needs to be a mana source, so the ONLY mana is the
	// pool below -- the bound is exactly the pool's arithmetic.
	battlePerm(t, e, 0, xArtisanBearSrc)
	ao := e.G.AddObject(artisan, 0)
	ao.Zone = state.ZBattlefield
	e.G.SetZone(state.ZBattlefield, 0, append(e.G.Zone(state.ZBattlefield, 0), ao.ID))
	artisanID := ao.ID
	// {G}{G}: X=1 prices at 1 generic + 1 pip = 2 units (payable), X=2 at 3
	// units (not payable). The precondition matters: a pool of 3 or more
	// would make X=2 legitimately offered and mute the assertion.
	addMana(t, e, 0, "GG")
	if got := e.G.Players[0].Pool.Total(); got != 2 {
		t.Fatalf("precondition: pool = %d, want 2", got)
	}
	opt, ok := findAbilityOption(e, artisanID, 0)
	if !ok {
		t.Fatal("Fiend Artisan activation not offered from a {G}{G} pool")
	}
	submitChoices(t, e, opt.Index)
	d := e.Pending()
	if d == nil || d.Kind != decision.KChoose || len(d.Options) == 0 || d.Options[0].Kind != "x" {
		t.Fatalf("want X announcement, got %+v", d)
	}
	seen := map[int]bool{}
	for _, o := range d.Options {
		if o.Kind == "x" {
			seen[o.Amount] = true
		}
	}
	if seen[2] || len(seen) != 2 {
		t.Fatalf("X options %+v: X=2 was priced against the pip-free {X} and is unpayable once the {B/G} pip needs a third unit; want exactly X = 0 and X = 1", d.Options)
	}
	if !seen[0] || !seen[1] {
		t.Fatalf("X options %+v: X = 0 and X = 1 must stay offered", d.Options)
	}
	// Announce X=1 and let the flow settle: the pip face is {G} (the pool's
	// only colour), the walker taps and the bear is sacrificed, so the
	// activation must resolve with no abort Note.
	submitChoices(t, e, 1)
	for i := 0; i < 40; i++ {
		dd := e.Pending()
		if dd == nil {
			if len(e.G.Stack) == 0 && len(e.pendingTriggers) == 0 {
				break
			}
			e.priorityRound()
			continue
		}
		if dd.Kind == decision.KPriority {
			if len(e.G.Stack) == 0 && len(e.pendingTriggers) == 0 {
				break
			}
			idx := -1
			for _, o := range dd.Options {
				if o.Kind == "pass" {
					idx = o.Index
				}
			}
			if idx < 0 {
				t.Fatalf("priority decision with no pass option: %+v", dd.Options)
			}
			submitChoices(t, e, idx)
			continue
		}
		if len(dd.Options) == 0 {
			t.Fatalf("empty decision: %+v", dd)
		}
		submitChoices(t, e, dd.Options[0].Index)
	}
	if hasNote(e, "no longer payable") {
		t.Fatal("abort Note: the X=1 announcement must have resolved")
	}
	if o := e.G.Obj(artisanID); o.Zone != state.ZBattlefield || !o.Tapped {
		t.Fatalf("Fiend Artisan zone %s tapped %v, want battlefield tapped", o.Zone, o.Tapped)
	}
	for _, id := range e.G.Zone(state.ZBattlefield, 0) {
		if o := e.G.Obj(id); o != nil && o.Face() != nil && o.Face().Name == "Bear" {
			t.Fatalf("Bear still on the battlefield: the sacrifice part did not settle")
		}
	}
}

const xArtisanBearSrc = "Name:Bear\nManaCost:G\nTypes:Creature Bear\nPT:2/2\nOracle:x\n"
