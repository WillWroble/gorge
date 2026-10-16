package bot

import (
	"github.com/adams-shaun/gorge/botpolicy"
	"github.com/adams-shaun/gorge/bots"
	"github.com/adams-shaun/gorge/bots/sanity"
	"github.com/adams-shaun/gorge/seat"
)

func init() {
	bots.Register(bots.Entry{Info: bots.Info{
		Name: "bot", Label: "Bot", Description: "The production baseline policy. On auto-pay tables it uses combat simulation for attacks and blocks.", Tier: bots.Production,
		Strength: []bots.Measurement{{Claim: "53.17% [52.68, 53.65] constructed and 53.69% [52.73, 54.64] commander vs the plain auto-pay bot", Versus: "plain auto-pay bot", Setting: "held-out auto-pay tables", Source: "commit 66aa546dd"}},
		Cost:     bots.Cost{Scope: "per decision", Note: "not measured; INFERRED <1 ms (bot vs bot ~550k games/h on 8 workers)", Source: "docs/superpowers/specs/2026-09-28-hosted-bot-packages.md §3.1"},
		Formats:  []string{"constructed", "commander"}, Caretaker: "bot",
	}, New: newBot})
}

func newBot(o bots.Options) (seat.Seat, error) {
	var s seat.Seat
	if o.AutoPayMana {
		s = seat.NewAttackSimBot(o.Seed, botpolicy.DefaultAttackSimParams()).EnableAutoPayMana()
	} else {
		s = seat.NewBot(o.Seed)
	}
	if o.CommonSense {
		return sanity.Wrap(s), nil
	}
	return s, nil
}
