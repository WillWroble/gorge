package draftbelief

import (
	"math"
	"math/rand/v2"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draftfeat"
)

// The pool-commitment model. A drafter's colour preference is not a fixed
// latent pair but its pool so far: the rating-colour drafter (and, to a
// first approximation, a person) scores a card v_c + κ_k·fit(c, pool),
// fit being the mean over the card's colours of each colour's share of the
// picks so far (a colourless card a fixed 0.5). Who took which card of a
// removed set then depends on each seat's pool at that time, which depends
// on the other sets' assignments. MeanField solves that fixed point: each
// seat's expected colour weights from the current per-set order
// posteriors, then every set's order posterior from those weights, damped,
// until it settles. SampleMF draws orders from the per-set posteriors and
// unseen picks from the filler law under the seat's final colour shares.

// PoolModel tunes the mean field.
type PoolModel struct {
	Beta, Kappa float64 // in the table's z units; KappaRamp from the table
	Iters       int
	Damp        float64 // weight of the new posterior per iteration
}

// DefaultPool is the pool model the report measured.
var DefaultPool = PoolModel{Beta: 4, Kappa: 2, Iters: 30, Damp: 0.5}

type mfState struct {
	q     [][]float64        // per set: posterior over gibbsSet.perms
	share [MaxOpp][5]float64 // final colour shares per seat
}

// pick is one expected pick by a seat: when, and the card's colour mask.
type pickAt struct {
	t    int
	mask uint8
	p    float64
}

// MeanField runs the pool model and stores its posteriors in the flow.
// It needs Exact() (every set enumerable).
func (f *Flow) MeanField(pm PoolModel) {
	g := f.prepare()
	ns := len(f.sets)
	st := &mfState{q: make([][]float64, ns)}
	for s := range f.sets {
		np := len(g.sets[s].perms)
		st.q[s] = make([]float64, np)
		for p := range st.q[s] {
			st.q[s][p] = 1 / float64(np)
		}
	}
	mask := func(ci int32) uint8 {
		if ci < 0 {
			return 0
		}
		return f.t.Cards[ci].Mask
	}
	for it := 0; it < pm.Iters; it++ {
		// Expected picks per seat with times.
		var picks [MaxOpp][]pickAt
		for s, rs := range f.sets {
			for p, perm := range g.sets[s].perms {
				w := st.q[s][p]
				if w <= 0 {
					continue
				}
				for m, j := range perm {
					o := rs.offs[m]
					picks[o-1] = append(picks[o-1], pickAt{t: rs.time[m], mask: mask(rs.s0[rs.rpos[j]]), p: w})
				}
			}
		}
		// New posteriors.
		for s, rs := range f.sets {
			gs := &g.sets[s]
			w := make([]float64, len(gs.perms))
			// Each seat's colour weights before its pick from this set.
			var cw [MaxOpp + 1][5]float64
			var tot [MaxOpp + 1]float64
			for m, o := range rs.offs {
				for _, pk := range picks[o-1] {
					if pk.t >= rs.time[m] {
						continue
					}
					tot[m] += pk.p
					for c := 0; c < 5; c++ {
						if pk.mask&(1<<c) != 0 {
							cw[m][c] += pk.p
						}
					}
				}
			}
			for p, perm := range gs.perms {
				used := make([]bool, len(rs.s0))
				x := 1.0
				for m, j := range perm {
					x *= f.piPool(rs.s0, used, rs.rpos[j], &cw[m], tot[m], rs.kap[m], pm)
					used[rs.rpos[j]] = true
				}
				w[p] = x
			}
			z := sumW(w)
			if z <= 0 {
				continue
			}
			for p := range w {
				st.q[s][p] = (1-pm.Damp)*st.q[s][p] + pm.Damp*w[p]/z
			}
		}
		if it == pm.Iters-1 {
			for o := 0; o < f.n-1; o++ {
				tt := 0.0
				var cw [5]float64
				for _, pk := range picks[o] {
					tt += pk.p
					for c := 0; c < 5; c++ {
						if pk.mask&(1<<c) != 0 {
							cw[c] += pk.p
						}
					}
				}
				for c := 0; c < 5; c++ {
					if tt > 0 {
						st.share[o][c] = cw[c] / tt
					}
				}
			}
		}
	}
	f.mf = st
	f.mfModel = pm
}

// piPool is the pool model's π(s0[pos] | s0 minus used, colour weights).
func (f *Flow) piPool(s0 []int32, used []bool, pos int, cw *[5]float64, tot, kap float64, pm PoolModel) float64 {
	scale := pm.Kappa / math.Max(f.t.Model.Kappa, 1e-9) // kap carries the table's ramp
	den, num := 0.0, 0.0
	for i, ci := range s0 {
		if used[i] {
			continue
		}
		x := math.Exp(pm.Beta * (f.val(ci) + kap*scale*poolFit(f.mask(ci), cw, tot)))
		den += x
		if i == pos {
			num = x
		}
	}
	if den <= 0 {
		return 0
	}
	return num / den
}

func (f *Flow) val(ci int32) float64 {
	if ci < 0 {
		return f.t.Model.VMissing
	}
	return f.t.Cards[ci].V
}

func (f *Flow) mask(ci int32) uint8 {
	if ci < 0 {
		return 0
	}
	return f.t.Cards[ci].Mask
}

// poolFit is the rating-colour drafter's colour fit against expected
// colour weights.
func poolFit(mask uint8, cw *[5]float64, tot float64) float64 {
	if mask == 0 {
		return 0.5
	}
	if tot <= 0 {
		return 0
	}
	s, n := 0.0, 0
	for c := 0; c < 5; c++ {
		if mask&(1<<c) != 0 {
			s += cw[c] / tot
			n++
		}
	}
	return s / float64(n)
}

// ShareMF is offset o's final expected colour shares under the pool model.
func (f *Flow) ShareMF(o int) [5]float64 { return f.mf.share[o-1] }

// PairMF is offset o's pair under the pool model: the pair of its two
// largest colour shares.
func (f *Flow) PairMF(o int) int {
	sh := f.mf.share[o-1]
	best, bv := 0, -1.0
	for a := 0; a < draftfeat.NPairs; a++ {
		v := sh[draftfeat.Pairs[a][0]] + sh[draftfeat.Pairs[a][1]]
		if v > bv {
			best, bv = a, v
		}
	}
	return best
}

// SampleMF draws k pools for offset target from the mean-field posterior
// (Sample's constraints apply). MeanField must have run.
func (f *Flow) SampleMF(target, k int, c *Constraints, r *rand.Rand) [][]limited.CardRef {
	g := f.prepare()
	cl := f.claim(c)
	st := f.mf
	sh := st.share[target-1]
	// Filler law under the target's final shares.
	fill := make([][]float64, len(f.slots))
	for i, sl := range f.slots {
		if sl.off != target {
			continue
		}
		gl := &g.slots[i]
		zv := 0.0
		scale := f.mfModel.Kappa / math.Max(f.t.Model.Kappa, 1e-9)
		var cw [5]float64 = sh
		for _, ci := range sl.vis {
			zv += math.Exp(f.mfModel.Beta * (f.val(ci) + sl.kap*scale*poolFit(f.mask(ci), &cw, 1)))
		}
		w := make([]float64, len(gl.cand))
		for j, ci := range gl.cand {
			x := math.Exp(f.mfModel.Beta * (f.val(ci) + sl.kap*scale*poolFit(f.mask(ci), &cw, 1)))
			w[j] = f.freq[ci] * x / (x + zv)
		}
		fill[i] = w
	}
	out := make([][]limited.CardRef, 0, k)
	for d := 0; d < k; d++ {
		var pool []limited.CardRef
		for s, rs := range f.sets {
			gs := &g.sets[s]
			w := make([]float64, len(gs.perms))
			for p, perm := range gs.perms {
				w[p] = st.q[s][p]
				for m, j := range perm {
					if o, ok := cl.inst[[2]int{s, j}]; ok && o != rs.offs[m] {
						w[p] = 0
						break
					}
				}
			}
			if sumW(w) <= 0 {
				copy(w, st.q[s])
			}
			perm := gs.perms[drawIdx(w, r)]
			for m, j := range perm {
				if rs.offs[m] == target {
					pool = append(pool, rs.refs[j])
				}
			}
		}
		for i, sl := range f.slots {
			if sl.off != target {
				continue
			}
			if name, ok := cl.slot[i]; ok {
				pool = append(pool, f.ref(name, d, i, "u"))
				continue
			}
			pool = append(pool, f.ref(f.t.Cards[g.slots[i].cand[drawIdx(fill[i], r)]].Name, d, i, "u"))
		}
		for i, name := range cl.extra[target-1] {
			pool = append(pool, f.ref(name, d, i, "x"))
		}
		out = append(out, pool)
	}
	return out
}
