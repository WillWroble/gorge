package draftbelief

import (
	"math"
	"math/rand/v2"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draftfeat"
)

// The exact sampler. With at most ExactSeats other seats (the hosted
// 4-seat pod has 3) the joint posterior over the other seats' latent deck
// pairs has at most 10^3 states, and given the pairs every removed set's
// order and every unseen pick are independent:
//
//	p(a) ∝ Π_o rej_o(a_o) · Π_sets Σ_σ Π_m π(c_σ(m) | S_{m−1}, a_{off_m})
//
// (a filler's law sums to one, so unseen picks drop out unless claimed).
// SampleExact draws a from p(a), then each set's order and each unseen pick
// given a: ancestral sampling, no chain to mix, no label-switching modes.

// ExactSeats is the most other seats the exact joint covers.
const ExactSeats = 3

// Exact reports whether the pod is small enough for SampleExact.
func (f *Flow) Exact() bool {
	if f.n-1 > ExactSeats {
		return false
	}
	g := f.prepare()
	for _, gs := range g.sets {
		if len(gs.perms) == 0 {
			return false
		}
	}
	return true
}

func pow10(k int) int {
	x := 1
	for i := 0; i < k; i++ {
		x *= draftfeat.NPairs
	}
	return x
}

// jointPost is p(a) over every joint pair state t (seat o's pair is digit
// o-1 of t in base NPairs), under the claims.
func (f *Flow) jointPost(cl claims) []float64 {
	g := f.prepare()
	no := f.n - 1
	nt := pow10(no)
	lp := make([]float64, nt)
	var a [MaxOpp]int
	for t := 0; t < nt; t++ {
		u := t
		for o := 0; o < no; o++ {
			a[o] = u % draftfeat.NPairs
			u /= draftfeat.NPairs
			lp[t] += g.rej[o][a[o]]
		}
	}
	for s := range g.sets {
		rs := &f.sets[s]
		gs := &g.sets[s]
		ok := make([]bool, len(gs.perms))
		any := false
		for p, perm := range gs.perms {
			ok[p] = true
			for m, j := range perm {
				if o, c := cl.inst[[2]int{s, j}]; c && o != rs.offs[m] {
					ok[p] = false
					break
				}
			}
			any = any || ok[p]
		}
		if !any {
			for p := range ok {
				ok[p] = true
			}
		}
		for t := 0; t < nt; t++ {
			u := t
			for o := 0; o < no; o++ {
				a[o] = u % draftfeat.NPairs
				u /= draftfeat.NPairs
			}
			sum := 0.0
			for p := range gs.perms {
				if !ok[p] {
					continue
				}
				x := 1.0
				for m := range gs.perms[p] {
					x *= gs.pi[p][m][a[rs.offs[m]-1]]
				}
				sum += x
			}
			lp[t] += math.Log(math.Max(sum, 1e-300))
		}
	}
	for i, sl := range f.slots {
		name, ok := cl.slot[i]
		if !ok {
			continue
		}
		gl := &g.slots[i]
		ci := int32(f.t.Index(name))
		pos := -1
		for k, c := range gl.cand {
			if c == ci {
				pos = k
				break
			}
		}
		if pos < 0 {
			continue
		}
		for t := 0; t < nt; t++ {
			ao := (t / pow10(sl.off-1)) % draftfeat.NPairs
			lp[t] += math.Log(math.Max(gl.w[ao][pos], 1e-300))
		}
	}
	mx := math.Inf(-1)
	for _, x := range lp {
		mx = math.Max(mx, x)
	}
	tot := 0.0
	for t := range lp {
		lp[t] = math.Exp(lp[t] - mx)
		tot += lp[t]
	}
	for t := range lp {
		lp[t] /= tot
	}
	return lp
}

// PairExact is offset o's marginal pair posterior under the exact joint.
func (f *Flow) PairExact(o int) [draftfeat.NPairs]float64 {
	var out [draftfeat.NPairs]float64
	cl := claims{inst: map[[2]int]int{}, slot: map[int]string{}}
	for t, p := range f.jointPost(cl) {
		out[(t/pow10(o-1))%draftfeat.NPairs] += p
	}
	return out
}

// SampleExact draws k pools for offset target from the exact joint
// (Sample's constraints apply). It needs Exact().
func (f *Flow) SampleExact(target, k int, c *Constraints, r *rand.Rand) [][]limited.CardRef {
	g := f.prepare()
	cl := f.claim(c)
	post := f.jointPost(cl)
	out := make([][]limited.CardRef, 0, k)
	st := &gstate{perm: make([]int, len(g.sets)), seq: make([][]int, len(g.sets)), fill: make([]int, len(g.slots))}
	for d := 0; d < k; d++ {
		t := drawIdx(post, r)
		for o := 0; o < f.n-1; o++ {
			st.a[o] = t % draftfeat.NPairs
			t /= draftfeat.NPairs
		}
		g.resample(st, cl, r)
		out = append(out, g.pool(st, target, cl, d))
	}
	return out
}
