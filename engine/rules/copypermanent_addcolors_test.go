package rules

import (
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/state"
)

// TestJollyBalloonManCopyAddsRedToItsOwnColours pins CopyPermanent AddColors$
// (CR 707.9b "in addition to its other colors"): the copy of a green Grizzly
// Bears is a green AND red Balloon Bear. Before AddColors$ was read the colour
// was silently dropped and the key surfaced as an unread-parameter Note.
func TestJollyBalloonManCopyAddsRedToItsOwnColours(t *testing.T) {
	t.Parallel()
	reg := searchTestRegistry(t)
	e, cfg := corpusEngineCfg(t, reg,
		[]*cards.Card{lookup(t, reg, "The Jolly Balloon Man"), lookup(t, reg, "Grizzly Bears")},
		[]*cards.Card{})
	jbm := moveByName(t, e, 0, "The Jolly Balloon Man", state.ZBattlefield)
	bear := moveByName(t, e, 0, "Grizzly Bears", state.ZBattlefield)

	addMana(t, e, 0, "C")
	e.priorityRound()
	opt := abilityOption(t, e, jbm, 0)
	submitChoices(t, e, opt.Index)
	answerKTarget(t, e, bear)
	passUntilStackEmpty(t, e, 20)

	cid := findTokenCopyOf(t, e, e.G.Obj(bear).Card, bear)
	d := e.Derived(cid)
	if !strings.Contains(d.Colors, "G") || !strings.Contains(d.Colors, "R") {
		t.Fatalf("copy colours = %q, want both G (printed) and R (AddColors$)", d.Colors)
	}
	if !copyHasTypeWord(d.Types, "Balloon") || !copyHasTypeWord(d.Types, "Bear") {
		t.Fatalf("copy types = %v, want Bear and Balloon", d.Types)
	}
	if c := e.Derived(bear).Colors; c != "G" {
		t.Fatalf("the original Bear's colours = %q, want G (the grant leaked off the copy)", c)
	}
	noCopyPermanentModNote(t, e, "AddColors", "AddTypes")
	replayCheck(t, e, cfg)
}
