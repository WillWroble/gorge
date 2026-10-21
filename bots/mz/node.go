package mz

import (
	"math"
	"slices"

	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/bots/mz/encode"
	"github.com/adams-shaun/gorge/engine/rules"
	"github.com/adams-shaun/gorge/engine/state"
)

// A node is one decision point of the search: MCTSNode. Every node, a
// micro-node included, holds its own engine at its decision (the staged
// picks are in it), so nothing is ever replayed. The engine is needed only
// to validate the node's children and is released once they all are.
type node struct {
	parent   *node
	children []*node
	// choice is the pick of the parent's atom that leads here: an option
	// index or atomic.Stop.
	choice int

	// Set by validation.
	validated bool
	eng       *rules.Engine
	unchecked int // children not yet validated
	player    state.PlayerID
	atom      atomic.Atom
	choices   []int
	terminal  bool
	winner    bool
	illegal   bool
	hash      encode.Hashes
	value     float64 // the leaf evaluation (networkScore)
	net       *online // the network call, online only
	noise     uint64  // the root's Dirichlet seed, until its priors use it

	visits int
	score  float64
	prior  float64
}

func (n *node) leaf() bool { return len(n.children) == 0 }

func (n *node) mean() float64 {
	if n.visits > 0 {
		return n.score / float64(n.visits)
	}
	return -1
}

// legal is isLegalState: a terminal node, or an expanded priority node.
func (n *node) legal() bool {
	return n.terminal || (len(n.children) > 0 && n.atom.Type == atomic.Priority)
}

// containsLegal is containsLegalNode: a legal node below n.
func (n *node) containsLegal() bool {
	for _, c := range n.children {
		if c.legal() || c.containsLegal() {
			return true
		}
	}
	return false
}

// selectChild is PUCT from the searching player's side: Q is negated at the
// opponent's nodes, an unvisited child's Q is 0, and a single child is taken
// without scoring.
func (n *node) selectChild(me state.PlayerID) *node {
	if len(n.children) == 1 {
		return n.children[0]
	}
	sign := 1.0
	if n.player != me {
		sign = -1
	}
	sqrtN := math.Sqrt(float64(n.visits))
	var best *node
	bestVal := math.Inf(-1)
	for _, c := range n.children {
		q := 0.0
		if c.visits > 0 {
			q = c.mean()
		}
		if v := sign*q + cPUCT*c.prior*sqrtN/float64(1+c.visits); v > bestVal {
			best, bestVal = c, v
		}
	}
	return best
}

func (n *node) expand() {
	n.children = make([]*node, len(n.choices))
	for i, c := range n.choices {
		n.children[i] = &node{parent: n, choice: c, prior: 1 / float64(len(n.choices))}
	}
	n.unchecked = len(n.children)
	if n.unchecked == 0 {
		n.eng = nil
	}
}

func (n *node) backprop(result float64, visits int, discount float64) {
	for x := n; x != nil; x = x.parent {
		x.visits += visits
		x.score += result
		result *= discount
	}
}

// prune removes child c, correcting the statistics up the tree, and prunes n
// too once it has no child left.
func (n *node) prune(c *node, discount float64) {
	i := slices.Index(n.children, c)
	if i < 0 {
		return
	}
	n.children = slices.Delete(n.children, i, i+1)
	c.parent = nil
	if len(n.children) > 0 || n.parent == nil {
		if c.visits > 0 {
			n.backprop(-c.score*discount, -c.visits, discount)
		}
		return
	}
	n.parent.prune(n, discount)
}

func (n *node) emancipate() {
	if n.parent != nil {
		n.parent.children = slices.DeleteFunc(n.parent.children, func(c *node) bool { return c == n })
		n.parent = nil
	}
}

// reset masks n: bestChild's treatment of a child with no legal state below.
func (n *node) reset() {
	n.children = nil
	n.score, n.visits = 0, 0
}

// scope is getPlayerScope: the top of the decision scope n lies in for p,
// climbing through p's nodes and nodes with a single child.
func (n *node) scope(p state.PlayerID) *node {
	for n.parent != nil && (n.parent.player == p || len(n.parent.children) == 1) {
		n = n.parent
	}
	return n
}

// matchInScope is getMatchingStateInScope: breadth first from n, an expanded
// node with functional hash h, descending only through p's nodes and nodes
// with a single child.
func (n *node) matchInScope(h uint64, p state.PlayerID) *node {
	for q := []*node{n}; len(q) > 0; q = q[1:] {
		c := q[0]
		if c.leaf() {
			continue
		}
		if c.hash.Functional == h {
			return c
		}
		if c.player == p || len(c.children) == 1 {
			q = append(q, c.children...)
		}
	}
	return nil
}

// match is getMatchingState: breadth first from n, an expanded node whose
// logical hash is h.
func (n *node) match(h uint64) *node {
	for q := []*node{n}; len(q) > 0; q = q[1:] {
		c := q[0]
		if c.leaf() {
			continue
		}
		if c.hash.Logical == h {
			return c
		}
		q = append(q, c.children...)
	}
	return nil
}

// ancestorOf reports whether n is a strict ancestor of m (isParentOf).
func (n *node) ancestorOf(m *node) bool {
	for x := m.parent; x != nil; x = x.parent {
		if x == n {
			return true
		}
	}
	return false
}

// childOfCommonAncestor is getChildOfCommonAncestor: n's ancestor (or n)
// whose parent is the common ancestor of n and m.
func (n *node) childOfCommonAncestor(m *node) *node {
	for x := n; ; x = x.parent {
		if x.parent == nil || x.parent == m || x.parent.ancestorOf(m) {
			return x
		}
	}
}

// bestChild masks the children with no legal state below them, then takes
// the most visited, or one drawn by sample when given; nil when none is
// left.
func (n *node) bestChild(sample func(*node) *node) *node {
	if len(n.children) == 1 {
		if c := n.children[0]; c.legal() || c.containsLegal() {
			return c
		}
		return nil
	}
	for _, c := range n.children {
		if !c.legal() && !c.containsLegal() {
			c.reset()
		}
	}
	if sample != nil {
		return sample(n)
	}
	var best *node
	for _, c := range n.children {
		if c.visits > 0 && (best == nil || c.visits > best.visits) {
			best = c
		}
	}
	return best
}

func (n *node) size() int {
	s := 1
	for _, c := range n.children {
		s += c.size()
	}
	return s
}

func (n *node) depth() int {
	d := 0
	for _, c := range n.children {
		d = max(d, c.depth())
	}
	return d + 1
}
