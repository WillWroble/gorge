package mz

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
)

// Bot is ComputerPlayerMCTS2 in offline mode: a seat.EngineSeat that answers
// each decision by searching each of its atoms in turn, reusing the tree
// from one search to the next.
type Bot struct {
	opt    Options
	me     state.PlayerID
	root   *node
	spares []rules.Spare
	// Stats counts the work of every search the bot ran.
	Stats Stats
}

// Stats counts search work.
type Stats struct {
	Searches   int // atoms searched
	Sims       int // iterations of the search loop that backed up a result
	Reused     int // searches that started from a node of the previous tree
	Rebuilt    int // searches run again from a new root (no best child)
	Illegal    int // nodes pruned as illegal
	Duplicates int // new nodes pruned as duplicates
	Replaced   int // old nodes pruned for a duplicate on a better path
	Elapsed    time.Duration
	Engine     time.Duration // of Elapsed, in the engine: copying a node and stepping it to the next
	Hash       time.Duration // of Elapsed, hashing nodes
}

// New returns a bot searching with opt.
func New(opt Options) *Bot { return &Bot{opt: opt} }

var errNeedsEngine = errors.New("mz: a perfect-information seat, driven through seat.EngineSeat")

// Decide refuses: the bot reads the whole game (DecideEngine).
func (b *Bot) Decide(context.Context, view.View, decision.Decision) (decision.Intent, error) {
	return decision.Intent{}, errNeedsEngine
}

// WantsPotentialCasts is autotap's action set: every cast all available
// mana can pay (seat.PotentialCastConsumer).
func (b *Bot) WantsPotentialCasts() bool { return !b.opt.ManualTap }

// DecideEngine answers d on e, the bot's own copy of the game: what the
// search makes no node for is answered directly, and every other atom is
// searched, its pick staged on e, until the answer is complete.
func (b *Bot) DecideEngine(_ context.Context, e *rules.Engine, d decision.Decision) (decision.Intent, error) {
	b.me = d.Player
	if !b.opt.ManualTap {
		e.EnsurePotentialCasts()
	}
	if in, ok := b.whole(e, e.Pending()); ok {
		return in, nil
	}
	for {
		a, done := atomic.Next(e.Pending())
		if done {
			break
		}
		c, ok := b.forced(e, e.Pending(), a)
		if !ok {
			best := b.choose(e)
			if best == nil {
				// ComputerPlayerMCTS.priority's "game failure".
				return decision.Intent{}, errors.New("mz: no legal child after a rebuild")
			}
			c = best.choice
		}
		if c == atomic.Stop && a.Subject == 0 {
			break
		}
		var err error
		if c == atomic.Stop {
			err = e.Decline(a.Subject)
		} else {
			err = e.Stage(c)
		}
		if err != nil {
			return decision.Intent{}, err
		}
	}
	return decision.Intent{Seq: d.Seq, Player: d.Player, Choices: slices.Clone(e.Pending().Staged)}, nil
}

// choose is getNextAction: a new root at e's state, the old tree's node
// with the same logical hash in its place if there is one, a search, and
// the best child, which roots the next search. With no best child the
// search runs again from the new root.
func (b *Bot) choose(e *rules.Engine) *node {
	b.Stats.Searches++
	fresh := &node{validated: true}
	b.fill(fresh, e.Clone())
	b.evaluate(fresh)
	fresh.expand()
	root := fresh
	if b.root != nil {
		if m := b.root.match(fresh.hash.Logical); m != nil {
			b.Stats.Reused++
			root = m
			if m.unchecked > 0 {
				m.eng = e.Clone()
			}
		}
	}
	root.emancipate()
	b.root = root
	best := b.search()
	if best == nil {
		b.Stats.Rebuilt++
		b.root = fresh
		best = b.search()
	}
	b.root = best
	return best
}

// search is applyMCTS: select a leaf, validate it (pruning an illegal or
// duplicate one), evaluate and expand it, back up a virtual loss; until the
// tree holds a legal priority node and the budget or the timeout is spent,
// or a hard limit is hit.
func (b *Bot) search() *node {
	root := b.root
	start := time.Now()
	defer func() { b.Stats.Elapsed += time.Since(start) }()
	discount := b.opt.BackpropDiscount
	for {
		el := time.Since(start)
		if el > hardTimeout || root.size() >= maxNodes || root.depth() >= maxDepth || root.leaf() {
			break
		}
		if root.containsLegal() && (el > b.opt.Timeout || root.visits >= b.opt.Budget) {
			break
		}
		cur := root
		for !cur.leaf() {
			cur = cur.selectChild(b.me)
		}
		if !cur.terminal {
			b.validate(cur)
			if cur.illegal {
				b.Stats.Illegal++
				cur.parent.prune(cur, discount)
				continue
			}
			if !cur.terminal && b.opt.PruneDuplicates && b.dedup(cur) {
				continue
			}
		}
		result := -1.0
		if !cur.terminal {
			b.evaluate(cur)
			cur.expand()
		} else if cur.winner {
			result = 1
		}
		cur.backprop(result, 1, discount)
		b.Stats.Sims++
	}
	return root.bestChild()
}

// dedup prunes cur, or the node it duplicates, when a node in cur's decision
// scope has its functional hash: at their common ancestor the path with
// more visits keeps the state, the old one on a tie. It reports whether cur
// was pruned.
func (b *Bot) dedup(cur *node) bool {
	discount := b.opt.BackpropDiscount
	p := cur.parent.player
	for m := cur.scope(p).matchInScope(cur.hash.Functional, p); m != nil; m = cur.scope(p).matchInScope(cur.hash.Functional, p) {
		oldPath, newPath := m.childOfCommonAncestor(cur), cur.childOfCommonAncestor(m)
		if newPath.parent == m || newPath.visits <= oldPath.visits {
			b.Stats.Duplicates++
			cur.backprop(m.mean(), 1, discount)
			cur.parent.prune(cur, discount)
			b.release(cur.eng)
			cur.eng = nil
			return true
		}
		b.Stats.Replaced++
		m.parent.prune(m, discount)
	}
	return false
}

// evaluate is MCTSNode2.evaluate offline: GameStateEvaluator3 at a priority
// node, the parent's value at a micro-node, backed up without a visit.
func (b *Bot) evaluate(n *node) {
	switch {
	case n.atom.Type == atomic.Priority:
		n.value = evaluate(n.eng, b.me)
	case n.parent != nil:
		n.value = n.parent.value
	}
	n.backprop(1+n.value, 0, b.opt.BackpropDiscount)
}
