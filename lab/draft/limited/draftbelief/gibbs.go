package draftbelief

import (
	"math"
	"math/rand/v2"

	"github.com/adams-shaun/gorge/lab/draft/limited"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
)

// The joint sampler. Independent per-set sampling (Sample with pairs drawn
// from draftfeat's marginals) lets one draw give a seat a white card from
// one set and a red card from the next; the true assignment is colour
// consistent across the whole draft. Gibbs alternates, per other seat o, a
// latent deck pair a_o and every removed set's order and unseen pick:
//
//	p(a, σ, u) ∝ Π_o rej_o(a_o) · Π_sets Π_m π(c_σ(m) | S_{m−1}, a_{off_m})
//	            · Π_slots p(u_i | a_{off_i})
//
// where rej_o is the rejection likelihood of the packs o passed the
// observer before its first sighting (draftfeat's L(a)), π the opponent
// pick model and p(u | a) the filler law. Every conditional is exact.

// gibbsSet is one removed set's precomputed order table.
type gibbsSet struct {
	perms [][]int
	// pi[p][m][a]: π of perm p's m-th card for a seat of pair a.
	pi [][][draftfeat.NPairs]float64
}

// gibbsSlot is one unseen pick's filler law per pair: the cumulative
// weights over cand.
type gibbsSlot struct {
	cand []int32
	w    [draftfeat.NPairs][]float64 // normalised per pair
}

type gibbs struct {
	f     *Flow
	sets  []gibbsSet
	slots []gibbsSlot
	rej   [MaxOpp][draftfeat.NPairs]float64 // log rejection likelihood
}

// prepare builds the Gibbs tables once per Flow.
func (f *Flow) prepare() *gibbs {
	f.gOnce.Do(func() {
		g := &gibbs{f: f}
		for s := range f.sets {
			rs := &f.sets[s]
			var gs gibbsSet
			if len(rs.refs) <= maxExact {
				nr := len(rs.refs)
				idx := make([]int, nr)
				for i := range idx {
					idx[i] = i
				}
				var rec func(d int)
				rec = func(d int) {
					if d == nr {
						p := append([]int(nil), idx...)
						rows := make([][draftfeat.NPairs]float64, nr)
						for a := 0; a < draftfeat.NPairs; a++ {
							used := make([]bool, len(rs.s0))
							for m := 0; m < nr; m++ {
								rows[m][a] = f.pi(rs.s0, used, rs.rpos[p[m]], a, rs.kap[m])
								used[rs.rpos[p[m]]] = true
							}
						}
						gs.perms = append(gs.perms, p)
						gs.pi = append(gs.pi, rows)
						return
					}
					for i := d; i < nr; i++ {
						idx[d], idx[i] = idx[i], idx[d]
						rec(d + 1)
						idx[d], idx[i] = idx[i], idx[d]
					}
				}
				rec(0)
			}
			g.sets = append(g.sets, gs)
		}
		for _, sl := range f.slots {
			var gl gibbsSlot
			for ci := range f.t.Cards {
				if f.freq[ci] > 0 {
					gl.cand = append(gl.cand, int32(ci))
				}
			}
			for a := 0; a < draftfeat.NPairs; a++ {
				zv := 0.0
				for _, ci := range sl.vis {
					zv += f.score(ci, a, sl.kap)
				}
				w := make([]float64, len(gl.cand))
				tot := 0.0
				for i, ci := range gl.cand {
					x := f.score(ci, a, sl.kap)
					w[i] = f.freq[ci] * x / (x + zv)
					tot += w[i]
				}
				for i := range w {
					w[i] /= tot
				}
				gl.w[a] = w
			}
			g.slots = append(g.slots, gl)
		}
		// Rejection likelihood: every upstream seat that passed a first
		// sighting's visible cards.
		beta := f.t.Model.Beta
		for _, sl := range f.slots {
			if f.noRej {
				break
			}
			o := sl.off
			ph := math.Exp(beta * f.t.Model.M[0])
			for a := 0; a < draftfeat.NPairs; a++ {
				sum := 0.0
				for _, ci := range sl.vis {
					sum += f.score(ci, a, sl.kap)
				}
				g.rej[o-1][a] += math.Log(ph / (ph + sum))
			}
		}
		f.g = g
	})
	return f.g
}

// GibbsOptions tunes SampleGibbs.
type GibbsOptions struct {
	Chains, Burn, Thin int
}

// DefaultGibbs is the sampler setting the report measured.
var DefaultGibbs = GibbsOptions{Chains: 4, Burn: 30, Thin: 3}

// SampleGibbs draws k pools for offset target from the joint posterior
// (Sample's constraints apply: a claimed card goes to its seat).
func (f *Flow) SampleGibbs(target, k int, c *Constraints, opt GibbsOptions, r *rand.Rand) [][]limited.CardRef {
	g := f.prepare()
	cl := f.claim(c)
	if opt.Chains < 1 {
		opt.Chains = 1
	}
	out := make([][]limited.CardRef, 0, k)
	per := (k + opt.Chains - 1) / opt.Chains
	for ch := 0; ch < opt.Chains && len(out) < k; ch++ {
		st := g.init(cl, r)
		for it := 0; it < opt.Burn; it++ {
			g.sweep(st, cl, r)
		}
		for d := 0; d < per && len(out) < k; d++ {
			for it := 0; it < max(opt.Thin, 1); it++ {
				g.sweep(st, cl, r)
			}
			out = append(out, g.pool(st, target, cl, len(out)))
		}
	}
	return out
}

type gstate struct {
	a    [MaxOpp]int
	perm []int   // per set: index into gibbsSet.perms (or -1 for a large set)
	seq  [][]int // large sets: the sampled order
	fill []int   // per slot: index into cand, -1 when forced
}

func (g *gibbs) init(cl claims, r *rand.Rand) *gstate {
	f := g.f
	st := &gstate{perm: make([]int, len(g.sets)), seq: make([][]int, len(g.sets)), fill: make([]int, len(g.slots))}
	for o := 1; o < f.n; o++ {
		st.a[o-1] = drawIdx(f.post[o-1][:], r)
	}
	g.resample(st, cl, r)
	return st
}

// sweep is one Gibbs pass: each seat's pair, then every set and slot.
func (g *gibbs) sweep(st *gstate, cl claims, r *rand.Rand) {
	f := g.f
	var lp [draftfeat.NPairs]float64
	for o := 1; o < f.n; o++ {
		for a := 0; a < draftfeat.NPairs; a++ {
			lp[a] = g.rej[o-1][a]
		}
		for s := range g.sets {
			rs := &f.sets[s]
			gs := &g.sets[s]
			if st.perm[s] < 0 {
				continue
			}
			rows := gs.pi[st.perm[s]]
			for m, off := range rs.offs {
				if off != o {
					continue
				}
				for a := 0; a < draftfeat.NPairs; a++ {
					lp[a] += math.Log(math.Max(rows[m][a], 1e-300))
				}
			}
		}
		for i, sl := range f.slots {
			if sl.off != o || st.fill[i] < 0 {
				continue
			}
			for a := 0; a < draftfeat.NPairs; a++ {
				lp[a] += math.Log(math.Max(g.slots[i].w[a][st.fill[i]], 1e-300))
			}
		}
		mx := math.Inf(-1)
		for _, x := range lp {
			mx = math.Max(mx, x)
		}
		var w [draftfeat.NPairs]float64
		for a := range w {
			w[a] = math.Exp(lp[a] - mx)
		}
		st.a[o-1] = drawIdx(w[:], r)
	}
	g.resample(st, cl, r)
}

// resample redraws every set's order and every unseen pick given st.a.
func (g *gibbs) resample(st *gstate, cl claims, r *rand.Rand) {
	f := g.f
	for s := range g.sets {
		rs := &f.sets[s]
		gs := &g.sets[s]
		if len(gs.perms) == 0 {
			allowed := func(m, j int) bool {
				o, ok := cl.inst[[2]int{s, j}]
				return !ok || o == rs.offs[m]
			}
			st.perm[s] = -1
			st.seq[s] = f.assignSeq(rs, &st.a, allowed, r, st.seq[s][:0])
			continue
		}
		w := make([]float64, len(gs.perms))
		for p, perm := range gs.perms {
			x := 1.0
			for m, j := range perm {
				if o, ok := cl.inst[[2]int{s, j}]; ok && o != rs.offs[m] {
					x = 0
					break
				}
				x *= gs.pi[p][m][st.a[rs.offs[m]-1]]
			}
			w[p] = x
		}
		st.perm[s] = drawIdx(w, r)
	}
	for i, sl := range f.slots {
		if _, ok := cl.slot[i]; ok {
			st.fill[i] = -1
			continue
		}
		st.fill[i] = drawIdx(g.slots[i].w[st.a[sl.off-1]], r)
	}
}

// pool is st's pool for target.
func (g *gibbs) pool(st *gstate, target int, cl claims, d int) []limited.CardRef {
	f := g.f
	var pool []limited.CardRef
	for s := range g.sets {
		rs := &f.sets[s]
		order := st.seq[s]
		if st.perm[s] >= 0 {
			order = g.sets[s].perms[st.perm[s]]
		}
		for m, j := range order {
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
		pool = append(pool, f.ref(f.t.Cards[g.slots[i].cand[st.fill[i]]].Name, d, i, "u"))
	}
	for i, name := range cl.extra[target-1] {
		pool = append(pool, f.ref(name, d, i, "x"))
	}
	return pool
}

// PairOf is the latent pair distribution of offset o under the Gibbs
// chain (for reporting): the share of n sweeps with each pair.
func (f *Flow) PairGibbs(o, n int, r *rand.Rand) [draftfeat.NPairs]float64 {
	g := f.prepare()
	var cl claims
	cl.inst, cl.slot = map[[2]int]int{}, map[int]string{}
	st := g.init(cl, r)
	for i := 0; i < DefaultGibbs.Burn; i++ {
		g.sweep(st, cl, r)
	}
	var out [draftfeat.NPairs]float64
	for i := 0; i < n; i++ {
		g.sweep(st, cl, r)
		out[st.a[o-1]] += 1 / float64(n)
	}
	return out
}
