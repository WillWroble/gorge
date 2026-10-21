package mz

import (
	"math/rand/v2"
	"slices"
	"time"

	"github.com/adams-shaun/gorge/bots/botpolicy"
	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/bots/mz/encode"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/events"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
)

// This file is MCTSPlayer: how a sim moves from one node to the next. A
// node's engine is its parent's clone with the node's pick applied; what
// the search makes no node for is then answered here until the next
// decision it does (validateState).

// validate fills in n from its parent's engine. An answer the engine
// refuses, a cast it reverses, or a panic marks n illegal (scriptFailed).
func (b *Bot) validate(n *node) {
	p := n.parent
	t := time.Now()
	e := b.clone(p.eng)
	if p.unchecked--; p.unchecked == 0 {
		p.eng = nil
	}
	n.validated = true
	defer func() {
		if recover() != nil {
			n.illegal, n.eng = true, nil
		}
	}()
	start := len(e.L.Events)
	failed := atomic.Apply(e, p.atom, n.choice) != nil || b.settle(e) != nil || reversed(e.L.Events[start:])
	b.Stats.Engine += time.Since(t)
	if failed {
		n.illegal = true
		b.release(e)
		return
	}
	b.fill(n, e)
	if n.terminal {
		b.release(e)
	}
}

// clone copies e into the storage of an engine released earlier.
func (b *Bot) clone(e *rules.Engine) *rules.Engine {
	var sp rules.Spare
	if n := len(b.spares); n > 0 {
		sp, b.spares = b.spares[n-1], b.spares[:n-1]
	}
	return e.CloneInto(&sp)
}

// release recycles e's storage. e must be an engine nothing was cloned
// from: a clone shares its parent's event history.
func (b *Bot) release(e *rules.Engine) {
	if len(b.spares) < maxSpares {
		b.spares = append(b.spares, e.Release())
	}
}

// fill records the decision e poses at n.
func (b *Bot) fill(n *node, e *rules.Engine) {
	if e.G.Over || e.Pending() == nil {
		n.terminal = true
		n.winner = !e.G.Draw && e.G.Winner == b.me
		return
	}
	d := e.Pending()
	n.eng = e
	n.player = d.Player
	n.atom, _ = atomic.Next(d)
	n.choices = b.choices(d, n.atom)
	t := time.Now()
	_, n.hash = b.enc.State(e, describe(e, d, n.atom))
	b.Stats.Hash += time.Since(t)
}

// describe is what the encoder is told about atom a of d (freezeState's
// arguments): its type, its text, its source (a combat atom's creature, as
// ChooseToAttackAbility's), and the cards only d offers.
func describe(e *rules.Engine, d *decision.Decision, a atomic.Atom) encode.Decision {
	ed := encode.Decision{Player: d.Player, Type: a.Type.String(), Text: d.Prompt, Source: d.Source}
	if a.Type == atomic.Priority {
		ed.Text = "priority"
	}
	if a.Subject != 0 {
		ed.Source = a.Subject
	}
	for _, i := range a.Options {
		if o := e.G.Obj(d.Options[i].Obj); o != nil && o.Zone == state.ZLibrary {
			ed.Pile = append(ed.Pile, d.Options[i].Obj)
		}
	}
	return ed
}

// settle answers e's pending decisions until one is a node.
func (b *Bot) settle(e *rules.Engine) error {
	for !e.G.Over && e.Pending() != nil {
		if !b.opt.ManualTap {
			e.EnsurePotentialCasts()
		}
		d := e.Pending()
		if in, ok := b.whole(e, d); ok {
			if err := e.Submit(in); err != nil {
				return err
			}
			continue
		}
		a, done := atomic.Next(d)
		if done {
			if err := e.Commit(); err != nil {
				return err
			}
			continue
		}
		c, ok := b.forced(e, d, a)
		if !ok {
			return nil
		}
		if err := atomic.Apply(e, a, c); err != nil {
			return err
		}
	}
	return nil
}

// whole answers a decision the search never makes a node for:
//   - the payment windows (CR 601.2g and the unless-cost windows): autotap
//     pays with gorge's bot's tapper, as XMage's auto-tapper pays; manual
//     tap pays from the pool only (autoPayFromPool);
//   - the starting player: the chooser starts;
//   - a mulligan when mulligans are off, the hand is under 6, or it is the
//     opponent's (XMage's sims assume the opponent keeps).
func (b *Bot) whole(e *rules.Engine, d *decision.Decision) (decision.Intent, bool) {
	in := decision.Intent{Seq: d.Seq, Player: d.Player}
	pick := func(match func(o decision.Option) bool) (decision.Intent, bool) {
		i := slices.IndexFunc(d.Options, match)
		in.Choices = []int{i}
		return in, i >= 0
	}
	switch {
	case encode.PaymentWindow(d) && b.opt.ManualTap:
		return pick(func(o decision.Option) bool { return o.Kind == "done" })
	case encode.PaymentWindow(d):
		brd := botpolicy.NewBoard(len(e.G.Players))
		return botpolicy.Decide(botpolicy.BoardFromGameInto(e.G, e, d.Player, &brd), d, rand.New(rand.NewPCG(0, 0))), true
	case d.Kind == decision.KStartingPlayer:
		return pick(func(o decision.Option) bool { return o.Player == d.Player })
	case d.Kind == decision.KMulligan && d.Max == 1 &&
		(!b.opt.Mulligans || d.Player != b.me || len(e.G.Zone(state.ZHand, d.Player)) < 6):
		return pick(func(o decision.Option) bool { return o.Kind == "keep" })
	}
	return in, false
}

// forced is an atom's only choice, when the search makes no node for it: a
// single choice, or a priority whose only playable is pass at a
// non-checkpoint (the auto-pass).
func (b *Bot) forced(e *rules.Engine, d *decision.Decision, a atomic.Atom) (int, bool) {
	cs := b.choices(d, a)
	if len(cs) != 1 || (a.Type == atomic.Priority && b.checkpoint(e)) {
		return 0, false
	}
	return cs[0], true
}

// choices are the picks a node offers for atom a: stop first, then the
// options. A priority's are its playables: never concede, and under autotap
// no mana ability. CHOOSE_NUM is capped as makeChoiceAmount caps it.
func (b *Bot) choices(d *decision.Decision, a atomic.Atom) []int {
	var cs []int
	if a.CanStop {
		cs = append(cs, atomic.Stop)
	}
	for _, i := range a.Options {
		k := d.Options[i].Kind
		if a.Type == atomic.Priority && (k == "concede" || !b.opt.ManualTap && k == "activate") {
			continue
		}
		cs = append(cs, i)
	}
	if a.Type == atomic.ChooseNum && len(cs) > maxNum {
		cs = cs[:maxNum]
	}
	return cs
}

// checkpoint is GameImpl.isCheckPoint: every priority under manual tap;
// under autotap the turn-1 upkeep, beginning of combat, and the declare
// attackers and blockers steps.
func (b *Bot) checkpoint(e *rules.Engine) bool {
	switch s := e.G.Step; {
	case b.opt.ManualTap:
		return true
	case e.G.Turn == 1 && s == state.StepUpkeep:
		return true
	default:
		return s == state.StepBeginCombat || s == state.StepDeclareAttackers || s == state.StepDeclareBlockers
	}
}

// reversed reports a cast or activation the engine reversed (CR 733.1): it
// could not be paid, so the action that started it was not legal.
func reversed(evs []events.Event) bool {
	return slices.ContainsFunc(evs, func(ev events.Event) bool {
		return ev.Kind == events.MoveZone && ev.Text == "reversed"
	})
}
