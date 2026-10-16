package draftfeat

import "math"

// jointSeats is the most other seats the exact joint posterior covers;
// jointSize its tuple count (10^jointSeats).
const (
	jointSeats = 3
	jointSize  = 1000
)

// digit is seat row o's pair in joint tuple t.
func digit(t, o int) int {
	for ; o > 0; o-- {
		t /= NPairs
	}
	return t % NPairs
}

// jointScale multiplies a per-seat likelihood into the joint.
func (e *Extractor) jointScale(o int, lik *[NPairs]float64) {
	s := 0.0
	for t := 0; t < e.jn; t++ {
		e.jp[t] *= lik[digit(t, o)]
		s += e.jp[t]
	}
	e.jointNormalise(s)
}

func (e *Extractor) jointNormalise(s float64) {
	if s <= 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		for t := 0; t < e.jn; t++ {
			e.jp[t] = 1 / float64(e.jn)
		}
		return
	}
	for t := 0; t < e.jn; t++ {
		e.jp[t] /= s
	}
}

// jointWheel is the exact wheel update on the joint: each tuple's
// likelihood is Σ_σ Π_m π(c_σ(m) | S_{m−1}, a_m) over every assignment σ
// of the removed set to the downstream seats in order.
func (e *Extractor) jointWheel(nr int) {
	w := &e.w
	beta := e.t.Model.Beta
	ns := len(w.s0)
	for m := 0; m < nr; m++ {
		if cap(w.e[m]) < ns*NPairs {
			w.e[m] = make([]float64, ns*NPairs)
		}
		w.e[m] = w.e[m][:ns*NPairs]
		w.tot[m] = [NPairs]float64{}
		for pos, ci := range w.s0 {
			v, _, fit := e.card(ci)
			for a := 0; a < NPairs; a++ {
				x := math.Exp(beta * (v + w.kap[m]*fit[a]))
				w.e[m][pos*NPairs+a] = x
				w.tot[m][a] += x
			}
		}
	}
	// pi[σ][m][a]: seat m's probability of its card under assignment σ.
	var pi [6][jointSeats][NPairs]float64
	var perm [jointSeats]int
	for i := range perm {
		perm[i] = i
	}
	np := 0
	var rec func(d int)
	rec = func(d int) {
		if d == nr {
			for m := 0; m < nr; m++ {
				pos := w.rpos[perm[m]]
				for a := 0; a < NPairs; a++ {
					den := w.tot[m][a]
					for j := 0; j < m; j++ {
						den -= w.e[m][w.rpos[perm[j]]*NPairs+a]
					}
					pi[np][m][a] = w.e[m][pos*NPairs+a] / den
				}
			}
			np++
			return
		}
		for i := d; i < nr; i++ {
			perm[d], perm[i] = perm[i], perm[d]
			rec(d + 1)
			perm[d], perm[i] = perm[i], perm[d]
		}
	}
	rec(0)
	s := 0.0
	for t := 0; t < e.jn; t++ {
		var am [jointSeats]int
		for m := 0; m < nr; m++ {
			am[m] = digit(t, w.off[m]-1)
		}
		lik := 0.0
		for p := 0; p < np; p++ {
			x := 1.0
			for m := 0; m < nr; m++ {
				x *= pi[p][m][am[m]]
			}
			lik += x
		}
		e.jp[t] *= lik
		s += e.jp[t]
	}
	e.jointNormalise(s)
}

// jointMarginals writes the joint's per-seat marginals into q.
func (e *Extractor) jointMarginals(nr int) {
	for o := 0; o < nr; o++ {
		e.q[o] = [NPairs]float64{}
	}
	for t := 0; t < e.jn; t++ {
		x := e.jp[t]
		u := t
		for o := 0; o < nr; o++ {
			e.q[o][u%NPairs] += x
			u /= NPairs
		}
	}
}
