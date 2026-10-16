package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/state"
)

// lengDiscardAsk drives Mind Peel's discard under a lone Library of Leng and
// returns the engine, the discarded card and the posed "may" ask.
func lengDiscardAsk(t *testing.T) (*Engine, state.ObjID, *decision.Decision) {
	t.Helper()
	return discardAsk(t, "Library of Leng")
}

func discardAsk(t *testing.T, replacement string) (*Engine, state.ObjID, *decision.Decision) {
	t.Helper()
	reg := testutil.CorpusRegistry(t)
	e := layerEngine(t)
	leng := onBoardCard(t, e, 0, mustCorpusCard(t, reg, replacement))
	if o := e.G.Obj(leng); o == nil || o.Zone != state.ZBattlefield {
		t.Fatal("precondition: the replacement source must be on the battlefield")
	}
	castOnStack(t, e, reg, "Mind Peel", 1, 0)
	e.resolveTop()
	d := e.Pending()
	if d == nil || d.ResumeKind != "discard" || len(d.Options) == 0 {
		t.Fatalf("discard decision = %+v", d)
	}
	card := d.Options[0].Obj
	submitChoices(t, e, d.Options[0].Index)
	if o := e.G.Obj(card); o == nil || o.Zone != state.ZHand {
		t.Fatal("precondition: the discarded card must still be in hand while the may is asked")
	}
	rd := e.Pending()
	if rd == nil || rd.Kind != decision.KReplacement || len(rd.Options) != 2 ||
		rd.Options[0].Kind != "apply" || rd.Options[1].Kind != "decline" {
		t.Fatalf("lone Optional$ Moved replacement must pose apply/decline, got %+v", rd)
	}
	if rd.Player != 0 {
		t.Fatalf("may asked of seat %d, want the discarding controller 0", rd.Player)
	}
	return e, card, rd
}

func TestLoneOptionalMovedReplacementLengApply(t *testing.T) {
	t.Parallel()
	e, card, rd := lengDiscardAsk(t)
	submitChoices(t, e, rd.Options[0].Index)
	if o := e.G.Obj(card); o.Zone != state.ZLibrary {
		t.Fatalf("applied Leng: card in %v, want library", o.Zone)
	}
}

func TestLoneOptionalMovedReplacementLengDecline(t *testing.T) {
	t.Parallel()
	e, card, rd := lengDiscardAsk(t)
	submitChoices(t, e, rd.Options[1].Index)
	if o := e.G.Obj(card); o.Zone != state.ZGraveyard {
		t.Fatalf("declined Leng: card in %v, want graveyard", o.Zone)
	}
}

func TestLoneOptionalMovedReplacementBotAnswerValid(t *testing.T) {
	t.Parallel()
	e, _, rd := lengDiscardAsk(t)
	in := newTestBot(1).answer(e, rd)
	if err := rd.Validate(in); err != nil {
		t.Fatalf("bot answer rejected by Validate: %v (choices %v)", err, in.Choices)
	}
	if err := e.Submit(in); err != nil {
		t.Fatalf("engine rejected the bot's answer: %v", err)
	}
}

// Nephalia Academy: the same lone "may" for an opponent-caused discard
// (ValidCause$ SpellAbility.OppCtrl); Mind Peel is cast by seat 1.
func TestLoneOptionalMovedReplacementNephaliaAcademy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		choice int
		want   state.Zone
	}{{"apply", 0, state.ZLibrary}, {"decline", 1, state.ZGraveyard}} {
		t.Run(tc.name, func(t *testing.T) {
			e, card, rd := discardAsk(t, "Nephalia Academy")
			submitChoices(t, e, rd.Options[tc.choice].Index)
			if o := e.G.Obj(card); o.Zone != tc.want {
				t.Fatalf("%s: card in %v, want %v", tc.name, o.Zone, tc.want)
			}
		})
	}
}
