package rules

import (
	"testing"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// TestTargetHintsMatchTargetDecisionInGames is the consistency property: in
// seeded real-card games, every hinted priority option, submitted on a clone,
// that poses a KTarget decision next, poses one whose options split by
// controlling seat exactly as the hint says.
func TestTargetHintsMatchTargetDecisionInGames(t *testing.T) {
	t.Parallel()
	reg := testutil.CorpusRegistry(t)
	all := testutil.LegacyDeckNames()
	var hinted, compared, other int
	for g := 0; g+1 < len(all) && g < 12; g += 2 {
		names := []string{all[g], all[g+1]}
		decks := [][]*cards.Card{testutil.RepoDeck(t, reg, all[g]), testutil.RepoDeck(t, reg, all[g+1])}
		cfg := Config{Seed: uint64(9300 + g), Names: names, Decks: decks, Tokens: reg.Tokens, NameUniverse: reg.Universe()}
		e := New(cfg)
		e.Advance()
		b := newTestBot(uint64(g) + 5)
		for n := 0; !e.G.Over && e.Pending() != nil && n < 3000; n++ {
			d := e.Pending()
			if d.Kind == decision.KPriority {
				e.EnsureTargetHints()
				for _, o := range e.Pending().Options {
					h := o.TargetHints
					if !h.Present() {
						continue
					}
					hinted++
					c := e.Clone()
					if err := c.Submit(decision.Intent{Seq: d.Seq, Player: d.Player, Choices: []int{o.Index}}); err != nil {
						t.Fatalf("%s vs %s: submit %q: %v", names[0], names[1], o.Label, err)
					}
					td := c.Pending()
					if td == nil || td.Kind != decision.KTarget {
						other++
						continue
					}
					own, opp := 0, 0
					for _, to := range td.Options {
						if to.Player == d.Player {
							own++
						} else {
							opp++
						}
					}
					compared++
					if own != h.Own() || opp != h.Opp() {
						t.Errorf("%s vs %s turn %d: %q hint own=%d opp=%d, target decision own=%d opp=%d",
							names[0], names[1], e.G.Turn, o.Label, h.Own(), h.Opp(), own, opp)
					}
				}
			}
			if err := e.Submit(b.answer(e, e.Pending())); err != nil {
				t.Fatalf("%s vs %s: %v", names[0], names[1], err)
			}
		}
	}
	t.Logf("hinted options %d, compared with the posed target decision %d, next decision not a target ask %d", hinted, compared, other)
	if compared == 0 {
		t.Fatal("no hinted option reached a target decision: the sample proves nothing")
	}
}
