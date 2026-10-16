package draftbelief

import (
	"fmt"
	"io"
	"math"
	"math/rand/v2"

	"github.com/adams-shaun/gorge/lab/draft/limited"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
)

// Debug prints every removed set with its unconditional marginals; owner
// maps a CardRef ID to its true offset (scoring only, never an input).
func (f *Flow) Debug(w io.Writer, owner func(id string) int) {
	for s, rs := range f.sets {
		fmt.Fprintf(w, "set %d offs %v |s0|=%d\n", s, rs.offs, len(rs.s0))
		for j, c := range rs.refs {
			ci := f.t.Index(c.Name)
			v, mask := -9.0, uint8(0)
			if ci >= 0 {
				v, mask = f.t.Cards[ci].V, f.t.Cards[ci].Mask
			}
			fmt.Fprintf(w, "  %-32s v=%5.2f mask=%05b true=%d marg=%.2f\n", c.Name, v, mask, owner(c.ID), rs.marg[j][:f.n-1])
		}
	}
}

// TrueOrderLogLik is Σ over exactly enumerated removed sets of
// log P(true order | pairs): how well the pick model, given the seats' true
// pairs, picks out who took what. Calibration only (owner is the truth).
func (f *Flow) TrueOrderLogLik(owner func(id string) int, pairs [MaxOpp]int) (ll float64, n int) {
	g := f.prepare()
	for s := range f.sets {
		rs := &f.sets[s]
		gs := &g.sets[s]
		if len(gs.perms) <= 1 {
			continue
		}
		tot, tru := 0.0, 0.0
		for p, perm := range gs.perms {
			x := 1.0
			match := true
			for m, j := range perm {
				x *= gs.pi[p][m][pairs[rs.offs[m]-1]]
				if owner(rs.refs[j].ID) != rs.offs[m] {
					match = false
				}
			}
			tot += x
			if match {
				tru = x
			}
		}
		if tot > 0 {
			ll += math.Log(math.Max(tru/tot, 1e-12))
			n++
		}
	}
	return ll, n
}

// JointLogOdds is log p(truth pairs) − log p(MAP pairs) under the exact
// joint, and the MAP state (debug only).
func (f *Flow) JointLogOdds(pairs [MaxOpp]int) (float64, [MaxOpp]int) {
	post := f.jointPost(claims{inst: map[[2]int]int{}, slot: map[int]string{}})
	t := 0
	for o := f.n - 2; o >= 0; o-- {
		t = t*10 + pairs[o]
	}
	b := 0
	for i := range post {
		if post[i] > post[b] {
			b = i
		}
	}
	var m [MaxOpp]int
	u := b
	for o := 0; o < f.n-1; o++ {
		m[o] = u % 10
		u /= 10
	}
	return math.Log(post[t]) - math.Log(post[b]), m
}

// SampleGivenPairs is SampleExact with the pairs fixed (an oracle
// ablation: the caller passes the true pairs to measure the ceiling of
// pair inference; never a legal input).
func (f *Flow) SampleGivenPairs(target, k int, pairs [MaxOpp]int, r *rand.Rand) [][]limited.CardRef {
	g := f.prepare()
	cl := f.claim(nil)
	out := make([][]limited.CardRef, 0, k)
	st := &gstate{a: pairs, perm: make([]int, len(g.sets)), seq: make([][]int, len(g.sets)), fill: make([]int, len(g.slots))}
	for d := 0; d < k; d++ {
		g.resample(st, cl, r)
		out = append(out, g.pool(st, target, cl, d))
	}
	return out
}

// TrueOrderMF is the mean field's mean posterior probability of each
// set's true order (calibration only).
func (f *Flow) TrueOrderMF(owner func(id string) int) (sum float64, n int) {
	g := f.prepare()
	for s := range f.sets {
		rs := &f.sets[s]
		gs := &g.sets[s]
		if len(gs.perms) <= 1 {
			continue
		}
		for p, perm := range gs.perms {
			match := true
			for m, j := range perm {
				if owner(rs.refs[j].ID) != rs.offs[m] {
					match = false
				}
			}
			if match {
				sum += math.Log(math.Max(f.mf.q[s][p], 1e-12))
				n++
			}
		}
	}
	return sum, n
}

// OwnPickFit scores the pool model on the observer's OWN picks (whose pool
// is known exactly): the share of picks it ranks first and the mean log
// probability it gives them. Calibration of the opponent model on seats
// whose picks are public in a simulation.
func OwnPickFit(o Obs, t *draftfeat.SetTable, pm PoolModel) (top1, ll float64, n int) {
	var cw [5]float64
	tot := 0.0
	ramp := t.Model.KappaRamp
	for _, sp := range o.Packs {
		kap := pm.Kappa
		if sp.Round == 1 && ramp > 0 && sp.PickNo <= ramp {
			kap *= float64(sp.PickNo-1) / float64(ramp)
		}
		best, bi, ti := math.Inf(-1), -1, -1
		den := 0.0
		xs := make([]float64, len(sp.Cards))
		for i, c := range sp.Cards {
			ci := int32(t.Index(c.Name))
			v, m := t.Model.VMissing, uint8(0)
			if ci >= 0 {
				v, m = t.Cards[ci].V, t.Cards[ci].Mask
			}
			s := v + kap*poolFit(m, &cw, tot)
			xs[i] = s
			if s > best {
				best, bi = s, i
			}
			if c.ID == sp.Took {
				ti = i
			}
		}
		for _, s := range xs {
			den += math.Exp(pm.Beta * (s - best))
		}
		if ti >= 0 && len(sp.Cards) > 1 {
			if bi == ti {
				top1++
			}
			ll += pm.Beta*(xs[ti]-best) - math.Log(den)
			n++
		}
		if ti >= 0 {
			ci := t.Index(sp.Cards[ti].Name)
			if ci >= 0 {
				tot++
				for c := 0; c < 5; c++ {
					if t.Cards[ci].Mask&(1<<c) != 0 {
						cw[c]++
					}
				}
			} else {
				tot++
			}
		}
	}
	return top1, ll, n
}
