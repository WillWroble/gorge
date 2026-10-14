package draftfeat

import (
	"math"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draft"
)

// table fills group T. It replays every visible pack in pick order from a
// uniform prior: a round boundary mixes the posteriors toward uniform, a
// first sighting (pick k <= n) applies the rejection likelihood to the k−1
// upstream seats, and a wheel (a pack seen at pick k back at pick k+n, same
// IDs) applies the exact assignment update to the n−1 downstream seats.
func (e *Extractor) table(f *Features, v draft.PickView, n int) {
	q := &e.q
	*q = [MaxSeats - 1][NPairs]float64{}
	for o := 0; o < n-1; o++ {
		for a := range q[o] {
			q[o][a] = 1.0 / NPairs
		}
	}
	e.joint = n-1 <= jointSeats
	if e.joint {
		e.jn = 1
		for o := 0; o < n-1; o++ {
			e.jn *= NPairs
		}
		for t := 0; t < e.jn; t++ {
			e.jp[t] = 1 / float64(e.jn)
		}
	}
	cur := len(v.Seen)
	lastFirst := -1
	for p := 0; p <= cur; p++ {
		r, k := int(e.round[p]), int(e.pick[p])
		if p > 0 && e.round[p-1] != e.round[p] {
			c := e.t.Model.Carry
			for t := 0; t < e.jn && e.joint; t++ {
				e.jp[t] = c*e.jp[t] + (1-c)/float64(e.jn)
			}
			for o := 0; o < n-1; o++ {
				for a := range q[o] {
					q[o][a] = c*q[o][a] + (1-c)/NPairs
				}
			}
		}
		d := direction(r)
		if k <= n {
			lastFirst = p
			if k > 1 {
				e.reject(p, n, d, r, k)
			}
			continue
		}
		i := p - n
		if i < 0 || i >= len(v.Picks) || e.round[i] != e.round[p] || int(e.pick[i]) != k-n {
			continue
		}
		later := v.Pack
		if p < cur {
			later = v.Seen[p]
		}
		if !e.wheelSets(v.Seen[i], later, v.Picks[i].ID, i, n) {
			continue
		}
		f.HaveWheel = true
		f.WheelMass = [NColours]float64{}
		for _, pos := range e.w.rpos {
			_, sh, _ := e.card(e.w.s0[pos])
			for c := 0; c < NColours; c++ {
				f.WheelMass[c] += sh[c]
			}
		}
		for m := 0; m < n-1; m++ {
			e.w.off[m] = mod((m+1)*d, n)
			e.w.kap[m] = e.kappa(r, k-n+m+1)
		}
		if e.joint {
			e.jointWheel(n - 1)
		} else {
			e.wheelUpdate(n - 1)
		}
	}
	if e.joint {
		e.jointMarginals(n - 1)
	}
	for o := 0; o < n-1; o++ {
		f.Posterior[o] = q[o]
	}
	e.lateness(f, lastFirst)
	e.avail(f, v, n)
}

// reject multiplies the rejection likelihood of pack p's visible set into
// each upstream seat that passed it: L(a) = e^{βm} / (e^{βm} + Σ_V e^{β(v+κ·fit)}).
func (e *Extractor) reject(p, n, d, r, k int) {
	idx := e.packIdx(p)
	beta := e.t.Model.Beta
	for m := 1; m < k && m < n; m++ {
		o := mod(-m*d, n)
		row := &e.q[o-1]
		kp := k - m
		kap := e.kappa(r, kp)
		phantom := math.Exp(beta * e.mk(kp))
		var lik [NPairs]float64
		s := 0.0
		for a := 0; a < NPairs; a++ {
			sum := 0.0
			for _, ci := range idx {
				sum += e.score(ci, a, kap)
			}
			lik[a] = phantom / (phantom + sum)
			row[a] *= lik[a]
			s += row[a]
		}
		normalise(row, s)
		if e.joint {
			e.jointScale(o-1, &lik)
		}
	}
}

func normalise(row *[NPairs]float64, s float64) {
	if s <= 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		for a := range row {
			row[a] = 1.0 / NPairs
		}
		return
	}
	for a := range row {
		row[a] /= s
	}
}

// wheelSets checks that later is seen (pack i minus own pick minus exactly
// n−1 cards, all by ID) and loads the wheel scratch: s0 = pack i after the
// own pick, rpos = positions in s0 of the removed set R.
func (e *Extractor) wheelSets(seen, later []limited.CardRef, own string, i, n int) bool {
	if len(seen)-1-len(later) != n-1 || n-1 > maxRemoved {
		return false
	}
	idx := e.packIdx(i)
	w := &e.w
	w.s0 = w.s0[:0]
	w.rpos = w.rpos[:0]
	ownSeen, matched := false, 0
	for j, c := range seen {
		if c.ID == own && !ownSeen {
			ownSeen = true
			continue
		}
		in := false
		for _, l := range later {
			if l.ID == c.ID {
				in = true
				break
			}
		}
		if in {
			matched++
		} else {
			w.rpos = append(w.rpos, len(w.s0))
		}
		w.s0 = append(w.s0, idx[j])
	}
	return ownSeen && matched == len(later) && len(w.rpos) == n-1
}

// maxRemoved bounds a wheel's removed set (n−1 for the largest pod).
const maxRemoved = MaxSeats - 1

// wheelScratch is the exact wheel update's working set.
type wheelScratch struct {
	s0   []int32 // table rows of the pack after the own pick
	rpos []int   // positions in s0 of the removed cards
	off  [maxRemoved]int
	kap  [maxRemoved]float64
	e    [maxRemoved][]float64 // e[m][pos*NPairs+a] = exp(β(v+κ_m·fit))
	tot  [maxRemoved][NPairs]float64
	acc  [maxRemoved][NPairs]float64
	path [maxRemoved]int
	nr   int
}

// wheelUpdate is the exact posterior update over every assignment σ of
// the removed set R to the downstream seats m = 1..nr in order, seat m
// picking from the pack minus σ(1..m−1):
//
//	p(σ, a_1..a_nr) ∝ Π_m q_m(a_m)·π(c_σ(m) | S_{m−1}, a_m)
//
// marginalised per seat. It walks the assignment tree depth first, so a
// shared prefix is scored once: the per-seat marginal is the sum over tree
// nodes of (prefix product)·w_m(a)·(suffix sum), which equals the sum over
// all (nr)! leaves exactly.
func (e *Extractor) wheelUpdate(nr int) {
	w := &e.w
	w.nr = nr
	beta := e.t.Model.Beta
	ns := len(w.s0)
	for m := 0; m < nr; m++ {
		if cap(w.e[m]) < ns*NPairs {
			w.e[m] = make([]float64, ns*NPairs)
		}
		w.e[m] = w.e[m][:ns*NPairs]
		w.tot[m] = [NPairs]float64{}
		w.acc[m] = [NPairs]float64{}
		for pos, ci := range w.s0 {
			v, _, fit := e.card(ci)
			for a := 0; a < NPairs; a++ {
				x := math.Exp(beta * (v + w.kap[m]*fit[a]))
				w.e[m][pos*NPairs+a] = x
				w.tot[m][a] += x
			}
		}
	}
	if e.dfs(0, 0, 1) <= 0 {
		return
	}
	for m := 0; m < nr; m++ {
		row := &e.q[w.off[m]-1]
		s := 0.0
		for a := 0; a < NPairs; a++ {
			s += w.acc[m][a]
		}
		if s <= 0 {
			continue
		}
		for a := 0; a < NPairs; a++ {
			row[a] = w.acc[m][a] / s
		}
	}
}

// dfs returns the sum, over completions of the assignment below depth m,
// of Π_{m'≥m} Z_{m'}, accumulating each seat's marginal on the way.
func (e *Extractor) dfs(m int, used uint32, prefix float64) float64 {
	w := &e.w
	if m == w.nr {
		return 1
	}
	em := w.e[m]
	var den [NPairs]float64
	den = w.tot[m]
	for j := 0; j < m; j++ {
		base := w.path[j] * NPairs
		for a := 0; a < NPairs; a++ {
			den[a] -= em[base+a]
		}
	}
	q := &e.q[w.off[m]-1]
	total := 0.0
	for j := 0; j < w.nr; j++ {
		if used&(1<<j) != 0 {
			continue
		}
		pos := w.rpos[j]
		base := pos * NPairs
		var wt [NPairs]float64
		z := 0.0
		for a := 0; a < NPairs; a++ {
			wt[a] = q[a] * em[base+a] / den[a]
			z += wt[a]
		}
		w.path[m] = pos
		sub := e.dfs(m+1, used|1<<j, prefix*z)
		if sub > 0 {
			c := prefix * sub
			acc := &w.acc[m]
			for a := 0; a < NPairs; a++ {
				acc[a] += c * wt[a]
			}
		}
		total += z * sub
	}
	return total
}

// lateness fills T2 from the latest first sighting.
func (e *Extractor) lateness(f *Features, p int) {
	if p < 0 {
		return
	}
	k := int(e.pick[p])
	ek := 0.0
	if ex := e.t.Expected; len(ex) > 0 {
		if k-1 < len(ex) {
			ek = ex[k-1]
		} else {
			ek = ex[len(ex)-1]
		}
	}
	var seen [NColours]bool
	for _, ci := range e.packIdx(p) {
		if ci < 0 {
			continue
		}
		diff := e.t.Cards[ci].V - ek
		mask := e.t.Cards[ci].Mask
		for c := 0; c < NColours; c++ {
			if mask&(1<<c) == 0 {
				continue
			}
			f.LateSum[c] += diff
			if !seen[c] || diff > f.LateMax[c] {
				f.LateMax[c] = diff
				seen[c] = true
			}
		}
	}
}

// avail fills T4: for the current direction and next round's, the mean
// over the remaining picks' path lengths L of Π_{m≤L} (1 − τ·P_{u_m}(x)),
// u_m = s − m·δ (the own seat, met when a path wraps, is skipped).
func (e *Extractor) avail(f *Features, v draft.PickView, n int) {
	cur := len(v.Seen)
	k := int(e.pick[cur])
	size := k - 1 + len(v.Pack) // the round's pack size
	tau := e.t.Model.Tau
	// P[o][x]: seat o's take propensity for colour x (0..4) or pair x−5.
	var P [MaxSeats - 1][NAvail]float64
	for o := 0; o < n-1; o++ {
		q := &e.q[o]
		for a := 0; a < NPairs; a++ {
			for c := 0; c < NColours; c++ {
				if PairMask[a]&(1<<c) != 0 {
					P[o][c] += q[a]
				}
			}
			for b := 0; b < NPairs; b++ {
				P[o][NColours+b] += q[a] * float64(colourCount(PairMask[a]&PairMask[b])) / 2
			}
		}
	}
	d := direction(int(e.round[cur]))
	for di, delta := range [2]int{d, -d} {
		from := k + 1
		if di == 1 {
			from = 1
		}
		cnt := 0
		var sum [NAvail]float64
		for kp := from; kp <= size; kp++ {
			cnt++
			for x := 0; x < NAvail; x++ {
				prod := 1.0
				for m := 1; m <= kp-1; m++ {
					o := mod(-m*delta, n)
					if o == 0 {
						continue
					}
					prod *= 1 - tau*P[o-1][x]
				}
				sum[x] += prod
			}
		}
		for x := 0; x < NAvail; x++ {
			if cnt == 0 {
				f.Avail[di][x] = 1
			} else {
				f.Avail[di][x] = sum[x] / float64(cnt)
			}
		}
	}
}
