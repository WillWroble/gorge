package mz

import (
	"math"
	"math/rand/v2"

	"github.com/adams-shaun/gorge/bots/mz/atomic"
	"github.com/adams-shaun/gorge/bots/mz/encode"
	"github.com/adams-shaun/gorge/bots/mz/remote"
)

// This file is MCTSNode2's online path: a node's state goes to the network,
// and when its result comes back the value is backed up and its children
// get the policy's priors. Up to maxPending+1 evaluations are in flight; the
// search descends past a node only once its result is in.

// An evaluation is a node's network call in flight.
type evaluation struct {
	n  *node
	ch <-chan remote.Result
}

// online is the network part of a node, kept from its validation until its
// evaluation comes back.
type online struct {
	arrays  encode.Arrays
	actions []encode.Key // per choice: its graph node
	noBonus []bool       // per choice: a pass or a mana ability
	pending bool
}

// submit sends n's state to the network (MCTSNode2.evaluate).
func (b *Bot) submit(n *node) {
	for len(b.flight) > maxPending {
		b.await(b.flight[0].n)
	}
	n.net.pending = true
	b.flight = append(b.flight, evaluation{n: n, ch: b.net.Infer(n.net.arrays)})
}

// await applies evaluations as they arrive until n's is in.
func (b *Bot) await(n *node) {
	for n.net != nil && n.net.pending {
		ev := b.flight[0]
		b.flight = b.flight[1:]
		b.apply(ev.n, <-ev.ch)
	}
}

// drain applies every evaluation in flight.
func (b *Bot) drain() {
	for len(b.flight) > 0 {
		b.await(b.flight[0].n)
	}
}

// apply is the evaluation's callback: the head the node's atom reads (unless
// that prior is off, or it is the opponent's node and opponent priors are),
// the value from the searching player's side, backed up without a visit,
// then the priors.
func (b *Bot) apply(n *node, r remote.Result) {
	n.net.pending = false
	if r.Err != nil {
		// REMOTE EVAL FAILURE: back up something so the tree is not stuck.
		n.backprop(0, 0, b.opt.BackpropDiscount)
		return
	}
	var policy []float32
	mine := n.player == b.me || !b.opt.NoPolicyOpponent
	switch n.atom.Type {
	case atomic.Priority:
		if !b.opt.NoPolicyPriority && mine {
			policy = r.Priority
		}
	case atomic.ChooseTarget:
		if !b.opt.NoPolicyTarget && mine {
			policy = r.Target
		}
	case atomic.ChooseUse:
		if !b.opt.NoPolicyUse && mine {
			policy = r.Binary
		}
	}
	sign := 1.0
	if n.player != b.me {
		sign = -1
	}
	n.value = sign * float64(r.Value)
	n.backprop(1+n.value, 0, b.opt.BackpropDiscount)
	if policy != nil {
		b.setPriors(n, policy)
	}
}

// setPriors: a softmax of the children's logits at prior temperature, plus
// the prior bonus for every child but pass and mana abilities (with no
// renormalization), then, once, the root's Dirichlet noise.
func (b *Bot) setPriors(n *node, policy []float32) {
	if len(n.children) == 0 {
		return
	}
	logit := b.logits(n, policy)
	maxLogit := math.Inf(-1)
	for _, l := range logit {
		maxLogit = max(maxLogit, l)
	}
	sum := 0.0
	for i, c := range n.children {
		c.prior = math.Exp((logit[i] - maxLogit) / b.opt.PriorTemperature)
		sum += c.prior
	}
	for _, c := range n.children {
		c.prior /= sum
		if !n.net.noBonus[slot(n, c)] {
			c.prior += b.opt.PriorBonus
		}
	}
	if n.noise != 0 {
		r := rand.New(rand.NewPCG(n.noise, 0))
		dir := make([]float64, len(n.children))
		total := 0.0
		for i := range dir {
			dir[i] = gamma(r, noiseAlpha)
			total += dir[i]
		}
		for i, c := range n.children {
			c.prior = (1-b.opt.DirichletNoiseEps)*c.prior + b.opt.DirichletNoiseEps*dir[i]/total
		}
		n.noise = 0
	}
}

// logits are the children's scores: the policy at each child's graph node,
// or for a yes/no its (false, true) pair.
func (b *Bot) logits(n *node, policy []float32) []float64 {
	out := make([]float64, len(n.children))
	if n.atom.Type == atomic.ChooseUse {
		for i, c := range n.children {
			if n.net.actions[slot(n, c)] == encode.UseTrueKey {
				out[i] = float64(policy[1])
			} else {
				out[i] = float64(policy[0])
			}
		}
		return out
	}
	at := make(map[encode.Key]int, len(n.net.arrays.Order))
	for i, k := range n.net.arrays.Order {
		at[k] = i
	}
	for i, c := range n.children {
		out[i] = float64(policy[at[n.net.actions[slot(n, c)]]])
	}
	return out
}

// slot is child c's index in n's choices (children are pruned, choices
// are not).
func slot(n *node, c *node) int {
	for i, ch := range n.choices {
		if ch == c.choice {
			return i
		}
	}
	return 0
}

// gamma samples Gamma(alpha, 1) (Marsaglia and Tsang; alpha < 1 boosted).
func gamma(r *rand.Rand, alpha float64) float64 {
	if alpha < 1 {
		return gamma(r, alpha+1) * math.Pow(r.Float64(), 1/alpha)
	}
	d := alpha - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for {
		x := r.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := r.Float64()
		if math.Log(u) < 0.5*x*x+d-d*v+d*math.Log(v) {
			return d * v
		}
	}
}

// sample is bestChild's temperature selection: a child drawn with
// probability proportional to visits^(1/τ).
func (b *Bot) sample(n *node) *node {
	t := b.opt.SelectionTemperature
	logp := make([]float64, len(n.children))
	maxp := math.Inf(-1)
	for i, c := range n.children {
		logp[i] = math.Log(float64(c.visits)) / t
		maxp = max(maxp, logp[i])
	}
	if math.IsInf(maxp, -1) {
		return nil
	}
	sum := 0.0
	for i := range logp {
		logp[i] = math.Exp(logp[i] - maxp)
		sum += logp[i]
	}
	x, acc := b.rng.Float64(), 0.0
	for i, c := range n.children {
		acc += logp[i] / sum
		if x <= acc {
			return c
		}
	}
	return nil
}
