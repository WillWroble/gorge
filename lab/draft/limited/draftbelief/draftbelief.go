// Package draftbelief infers what the other seats of a draft pod drafted,
// from ONE seat's legal observation log only: every pack the seat was
// passed (as it stood), the seat's own picks, the pod size and the pass
// directions, plus the public cards that seat has seen other seats play.
// It never receives another seat's picks, pool or deck, nor any pod or
// server state: Obs is the whole input, and an Obs can be rebuilt from the
// seat's own PickViews (Obs.Add), which is what the draft already shows the
// seat (docs/superpowers/reports/2026-10-15-draft-belief.md, "information
// boundary").
//
// The model is card flow. A card in a pack the seat passed was taken by the
// seats downstream of it, one card per seat in pass order, before the pack
// came back (or ran out): the removed set R between two sightings of the
// same pack (by CardRef ID) is exactly the downstream seats' picks, and
// only the assignment of R's cards to those seats is uncertain. It is
// weighted by the opponent pick model of draftfeat (π(c | pack, a) ∝
// exp(β(v_c + κ·fit(c, a))) under each seat's colour-pair posterior from
// draftfeat's table reads). Cards an upstream seat took before the seat
// first saw a pack are unseen; they are filled from the set's collation
// frequencies weighted by the same pick model. Sampling an assignment for
// every removed set and a filler for every unseen pick gives one opponent
// pool; a caller-supplied builder turns pools into the deck list corpus a
// searchprobe.Belief is built from.
//
// The package is pure: no clock, no I/O, randomness only from the caller's
// *rand.Rand, and it imports only the limited vocabulary and draftfeat.
package draftbelief

import (
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"sync"

	"github.com/adams-shaun/gorge/lab/draft/limited"
	"github.com/adams-shaun/gorge/lab/draft/limited/draft"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
)

// MaxOpp is the most other seats a pod has.
const MaxOpp = draftfeat.MaxSeats - 1

// maxExact is the largest removed set whose seat assignment is enumerated
// exactly (5! = 120 orders); a larger one (an 8-seat pod's 7-card set) is
// sampled seat by seat from the pick model restricted to the set, an
// approximation of the same posterior.
const maxExact = 5

// SeenPack is one pack as the observing seat saw it, and what it took.
type SeenPack struct {
	Round, PickNo int
	Cards         []limited.CardRef // the pack on offer, in pack order
	Took          string            // the CardRef ID the seat took
}

// Obs is one seat's legal observation log of a draft: the pod size and,
// in pick order, every pack it was passed. Offsets elsewhere in this
// package are relative to the observing seat: offset o is the seat o
// places in round 1's pass direction ((seat+o) mod Seats), the same
// convention as draftfeat.Features.Posterior.
type Obs struct {
	Seats int
	Packs []SeenPack
}

// Add appends one pick: the view the seat was shown and the ID it took.
// It reads only the view's current pack, round and pick number, so an
// eventhost recorder can call it with exactly what it sent the seat.
func (o *Obs) Add(v draft.PickView, took string) {
	if o.Seats == 0 {
		o.Seats = v.Seats
	}
	o.Packs = append(o.Packs, SeenPack{Round: v.Round, PickNo: v.PickNo, Cards: append([]limited.CardRef(nil), v.Pack...), Took: took})
}

// view is the PickView of the last recorded pick (draftfeat's input).
func (o *Obs) view() draft.PickView {
	n := len(o.Packs)
	v := draft.PickView{Seats: o.Seats}
	if n == 0 {
		return v
	}
	last := o.Packs[n-1]
	v.Round, v.PickNo, v.Pack = last.Round, last.PickNo, last.Cards
	for _, p := range o.Packs[:n-1] {
		v.Seen = append(v.Seen, p.Cards)
		for _, c := range p.Cards {
			if c.ID == p.Took {
				v.Picks = append(v.Picks, c)
				break
			}
		}
	}
	return v
}

// removedSet is the cards the downstream seats took from one passed pack
// before it came back to the observer (or ran out).
type removedSet struct {
	s0   []int32           // table rows of the pack after the observer's pick
	refs []limited.CardRef // the removed cards
	rpos []int             // positions of refs in s0
	offs []int             // offs[m]: offset of the m-th seat to pick from it
	kap  []float64         // κ at that seat's pick
	time []int             // that seat's pick time: round*1000 + pick number
	// marg[j][o-1]: unconditional P(offset o took refs[j]).
	marg [][MaxOpp]float64
}

// unseenPick is one pick an upstream seat made from a pack before the
// observer first saw it: its identity is unknown, the pack it left is vis.
type unseenPick struct {
	off  int
	vis  []int32
	kap  float64
	time int // round*1000 + that seat's pick number
}

// Flow is one observation log's card-flow analysis. It is immutable once
// built; Sample reads it with the caller's rng.
type Flow struct {
	n     int
	t     *draftfeat.SetTable
	fit   [][draftfeat.NPairs]float64
	freq  []float64
	post  [MaxOpp][draftfeat.NPairs]float64
	sets  []removedSet
	slots []unseenPick
	own   []limited.CardRef

	noRej   bool
	types   [][]float64
	mf      *mfState
	mfModel PoolModel
	gOnce   sync.Once
	g       *gibbs
}

// Options tunes Analyze. Freq is the per-table-row expected copies of a
// card per pack (the collation, public set data); nil = uniform over the
// table's nonland rows.
type Options struct {
	Freq []float64
	// Types are alternative card-value tables (one value per table row)
	// a seat may draft by: the pool-model sampler gives every other seat a
	// latent type and lets its picks select it. Nil = the table's own V.
	Types [][]float64
	// NoReject drops the rejection likelihood of first sightings from the
	// joint (an ablation).
	NoReject bool
}

// Analyze builds the card flow of o against the public set table t.
func Analyze(o Obs, t *draftfeat.SetTable, opt Options) (*Flow, error) {
	n := o.Seats
	if n < 2 || n > draftfeat.MaxSeats {
		return nil, errors.New("draftbelief: pod size out of range")
	}
	if len(o.Packs) == 0 {
		return nil, errors.New("draftbelief: empty observation")
	}
	f := &Flow{n: n, t: t, freq: opt.Freq, noRej: opt.NoReject, types: opt.Types}
	if len(f.types) == 0 {
		v := make([]float64, len(t.Cards))
		for i := range t.Cards {
			v[i] = t.Cards[i].V
		}
		f.types = [][]float64{v}
	}
	f.fit = make([][draftfeat.NPairs]float64, len(t.Cards))
	for i := range t.Cards {
		f.fit[i] = fitRow(t.Cards[i].Mask)
	}
	if f.freq == nil {
		f.freq = make([]float64, len(t.Cards))
		for i := range t.Cards {
			if t.Cards[i].Types&draftfeat.TypeLand == 0 {
				f.freq[i] = 1
			}
		}
	}
	feat := draftfeat.Extract(o.view(), t)
	for i := 0; i < n-1; i++ {
		f.post[i] = feat.Posterior[i]
	}
	idx := func(cs []limited.CardRef) []int32 {
		out := make([]int32, len(cs))
		for i, c := range cs {
			out[i] = int32(t.Index(c.Name))
		}
		return out
	}
	for p, sp := range o.Packs {
		d := direction(sp.Round)
		k := sp.PickNo
		if !hasID(sp.Cards, sp.Took) {
			return nil, errors.New("draftbelief: a seen pack does not hold the card taken from it")
		}
		f.own = append(f.own, refByID(sp.Cards, sp.Took))
		// Upstream picks before the first sighting.
		if k <= n {
			vis := idx(sp.Cards)
			for m := 1; m < k; m++ {
				f.slots = append(f.slots, unseenPick{off: mod(-m*d, n), vis: vis, kap: f.kappa(sp.Round, k-m), time: sp.Round*1000 + k - m})
			}
		}
		var s0 []limited.CardRef
		for _, c := range sp.Cards {
			if c.ID != sp.Took {
				s0 = append(s0, c)
			}
		}
		if len(s0) == 0 {
			continue
		}
		var later []limited.CardRef
		found := false
		if q := p + n; q < len(o.Packs) && o.Packs[q].Round == sp.Round && o.Packs[q].PickNo == k+n {
			later, found = o.Packs[q].Cards, true
		}
		rs := removedSet{s0: idx(s0)}
		for j, c := range s0 {
			if found && hasID(later, c.ID) {
				continue
			}
			rs.refs = append(rs.refs, c)
			rs.rpos = append(rs.rpos, j)
		}
		switch {
		case found && len(rs.refs) != n-1:
			continue // not a clean wheel: ignore rather than mis-assign
		case !found && len(s0) >= n:
			continue // the pack would come back but the log ends first
		}
		for m := range rs.refs {
			rs.offs = append(rs.offs, mod((m+1)*d, n))
			rs.kap = append(rs.kap, f.kappa(sp.Round, k+m+1))
			rs.time = append(rs.time, sp.Round*1000+k+m+1)
		}
		f.sets = append(f.sets, rs)
	}
	for i := range f.sets {
		f.marginals(&f.sets[i])
	}
	return f, nil
}

// Posterior is the colour-pair posterior of offset o (1..n-1).
func (f *Flow) Posterior(o int) [draftfeat.NPairs]float64 { return f.post[o-1] }

// Instance is one observed card and the probability a given seat took it.
type Instance struct {
	Card limited.CardRef
	P    float64
}

// Taken lists every observed card offset o may have taken, with the
// unconditional probability it did, most likely first; Unseen is how many
// of o's picks the observer never saw.
func (f *Flow) Taken(o int) (taken []Instance, unseen int) {
	for _, rs := range f.sets {
		for j, c := range rs.refs {
			if p := rs.marg[j][o-1]; p > 0 {
				taken = append(taken, Instance{Card: c, P: p})
			}
		}
	}
	for _, s := range f.slots {
		if s.off == o {
			unseen++
		}
	}
	sort.SliceStable(taken, func(i, j int) bool { return taken[i].P > taken[j].P })
	return taken, unseen
}

// Constraints are public facts about the other seats' pools: Known[o-1]
// are card names offset o has been seen to own (cards it revealed in any
// game of the event), so the sample gives them to o.
type Constraints struct {
	Known [MaxOpp][]string
}

// claim forces one card instance (set s, removed index j) or one unseen
// slot to an offset.
type claims struct {
	inst  map[[2]int]int // (set, j) -> offset
	slot  map[int]string // slot index -> forced name
	extra [MaxOpp][]string
}

func (f *Flow) claim(c *Constraints) claims {
	cl := claims{inst: map[[2]int]int{}, slot: map[int]string{}}
	if c == nil {
		return cl
	}
	for o := 1; o < f.n; o++ {
		for _, name := range c.Known[o-1] {
			best, bs, bj := 0.0, -1, -1
			for s := range f.sets {
				for j, ref := range f.sets[s].refs {
					if ref.Name != name {
						continue
					}
					if _, taken := cl.inst[[2]int{s, j}]; taken {
						continue
					}
					if p := f.sets[s].marg[j][o-1]; p > best {
						best, bs, bj = p, s, j
					}
				}
			}
			if bs >= 0 {
				cl.inst[[2]int{bs, bj}] = o
				continue
			}
			placed := false
			for i, sl := range f.slots {
				if sl.off != o {
					continue
				}
				if _, ok := cl.slot[i]; ok {
					continue
				}
				cl.slot[i] = name
				placed = true
				break
			}
			if !placed {
				cl.extra[o-1] = append(cl.extra[o-1], name)
			}
		}
	}
	return cl
}

// Sample draws k pools for offset target: per draw, a colour pair for every
// other seat from its posterior, an assignment for every removed set and a
// card for every unseen pick, under the constraints. A filler card's ID is
// "u<draw>.<slot>"; a constraint's extra card's "x<draw>.<i>".
func (f *Flow) Sample(target, k int, c *Constraints, r *rand.Rand) [][]limited.CardRef {
	cl := f.claim(c)
	out := make([][]limited.CardRef, 0, k)
	var pairs [MaxOpp]int
	var perm []int
	for d := 0; d < k; d++ {
		for o := 1; o < f.n; o++ {
			pairs[o-1] = drawIdx(f.post[o-1][:], r)
		}
		var pool []limited.CardRef
		for s := range f.sets {
			rs := &f.sets[s]
			perm = f.assign(s, rs, &pairs, cl, r, perm[:0])
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
			ci := f.filler(sl, pairs[target-1], r)
			if ci < 0 {
				continue
			}
			pool = append(pool, f.ref(f.t.Cards[ci].Name, d, i, "u"))
		}
		for i, name := range cl.extra[target-1] {
			pool = append(pool, f.ref(name, d, i, "x"))
		}
		out = append(out, pool)
	}
	return out
}

func (f *Flow) ref(name string, d, i int, tag string) limited.CardRef {
	ci := f.t.Index(name)
	r := limited.CardRef{ID: tag + itoa(d) + "." + itoa(i), Name: name, Set: f.t.Set}
	if ci >= 0 {
		r.Rarity = f.t.Cards[ci].Rarity
	}
	return r
}

// assign draws one seat order for removed set s: perm[m] is the index in
// rs.refs of the card the m-th downstream seat took.
func (f *Flow) assign(s int, rs *removedSet, pairs *[MaxOpp]int, cl claims, r *rand.Rand, perm []int) []int {
	nr := len(rs.refs)
	allowed := func(m, j int) bool {
		o, ok := cl.inst[[2]int{s, j}]
		return !ok || o == rs.offs[m]
	}
	if nr > maxExact {
		return f.assignSeq(rs, pairs, allowed, r, perm)
	}
	ws := f.permWeights(rs, pairs, allowed)
	if sumW(ws.w) <= 0 {
		ws = f.permWeights(rs, pairs, func(int, int) bool { return true })
	}
	i := drawIdx(ws.w, r)
	return append(perm, ws.p[i]...)
}

type permSet struct {
	p [][]int
	w []float64
}

// permWeights enumerates every order of rs's cards with its likelihood
// Π_m π(c_σ(m) | S_{m−1}, a_{offs[m]}) (pairs given) or the pair-mixed
// likelihood (pairs nil).
func (f *Flow) permWeights(rs *removedSet, pairs *[MaxOpp]int, allowed func(m, j int) bool) permSet {
	nr := len(rs.refs)
	var ps permSet
	idx := make([]int, nr)
	for i := range idx {
		idx[i] = i
	}
	var rec func(d int)
	rec = func(d int) {
		if d == nr {
			w := 1.0
			used := make([]bool, len(rs.s0))
			for m := 0; m < nr; m++ {
				j := idx[m]
				if !allowed(m, j) {
					w = 0
					break
				}
				o := rs.offs[m]
				if pairs != nil {
					w *= f.pi(rs.s0, used, rs.rpos[j], pairs[o-1], rs.kap[m])
				} else {
					x := 0.0
					for a := 0; a < draftfeat.NPairs; a++ {
						x += f.post[o-1][a] * f.pi(rs.s0, used, rs.rpos[j], a, rs.kap[m])
					}
					w *= x
				}
				used[rs.rpos[j]] = true
			}
			ps.p = append(ps.p, append([]int(nil), idx...))
			ps.w = append(ps.w, w)
			return
		}
		for i := d; i < nr; i++ {
			idx[d], idx[i] = idx[i], idx[d]
			rec(d + 1)
			idx[d], idx[i] = idx[i], idx[d]
		}
	}
	rec(0)
	return ps
}

// assignSeq is the large-set approximation: seat by seat, a card drawn from
// the pick model restricted to the set's remaining allowed cards.
func (f *Flow) assignSeq(rs *removedSet, pairs *[MaxOpp]int, allowed func(m, j int) bool, r *rand.Rand, perm []int) []int {
	nr := len(rs.refs)
	used := make([]bool, len(rs.s0))
	taken := make([]bool, nr)
	w := make([]float64, nr)
	for m := 0; m < nr; m++ {
		o := rs.offs[m]
		for j := 0; j < nr; j++ {
			w[j] = 0
			if !taken[j] && allowed(m, j) {
				w[j] = f.pi(rs.s0, used, rs.rpos[j], pairs[o-1], rs.kap[m])
			}
		}
		if sumW(w) <= 0 {
			for j := 0; j < nr; j++ {
				if !taken[j] {
					w[j] = 1
				}
			}
		}
		j := drawIdx(w, r)
		taken[j] = true
		used[rs.rpos[j]] = true
		perm = append(perm, j)
	}
	return perm
}

// marginals fills rs.marg from the pair-mixed order likelihoods.
func (f *Flow) marginals(rs *removedSet) {
	rs.marg = make([][MaxOpp]float64, len(rs.refs))
	if len(rs.refs) > maxExact {
		for j := range rs.refs {
			for _, o := range rs.offs {
				rs.marg[j][o-1] += 1 / float64(len(rs.refs))
			}
		}
		return
	}
	ps := f.permWeights(rs, nil, func(int, int) bool { return true })
	tot := sumW(ps.w)
	if tot <= 0 {
		return
	}
	for i, p := range ps.p {
		for m, j := range p {
			rs.marg[j][rs.offs[m]-1] += ps.w[i] / tot
		}
	}
}

// pi is π(s0[pos] | s0 minus used, a, κ).
func (f *Flow) pi(s0 []int32, used []bool, pos, a int, kap float64) float64 {
	den := 0.0
	var num float64
	for i, ci := range s0 {
		if used[i] {
			continue
		}
		x := f.score(ci, a, kap)
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

func (f *Flow) score(ci int32, a int, kap float64) float64 {
	m := &f.t.Model
	if ci < 0 {
		return math.Exp(m.Beta * m.VMissing)
	}
	return math.Exp(m.Beta * (f.t.Cards[ci].V + kap*f.fit[ci][a]))
}

// filler draws an unseen pick: a table card weighted by its collation
// frequency times the chance a seat of pair a takes it over the pack it
// left (vis).
func (f *Flow) filler(sl unseenPick, a int, r *rand.Rand) int {
	zv := 0.0
	for _, ci := range sl.vis {
		zv += f.score(ci, a, sl.kap)
	}
	tot := 0.0
	w := make([]float64, len(f.t.Cards))
	for ci := range f.t.Cards {
		if f.freq[ci] <= 0 {
			continue
		}
		x := f.score(int32(ci), a, sl.kap)
		w[ci] = f.freq[ci] * x / (x + zv)
		tot += w[ci]
	}
	if tot <= 0 {
		return -1
	}
	return drawIdx(w, r)
}

// kappa is draftfeat's κ ramp.
func (f *Flow) kappa(round, k int) float64 {
	m := &f.t.Model
	if round > 1 || m.KappaRamp <= 0 || k > m.KappaRamp {
		return m.Kappa
	}
	return m.Kappa * float64(k-1) / float64(m.KappaRamp)
}

func fitRow(mask uint8) (out [draftfeat.NPairs]float64) {
	n := 0
	for i := 0; i < draftfeat.NColours; i++ {
		if mask&(1<<i) != 0 {
			n++
		}
	}
	for a := range out {
		if n == 0 {
			out[a] = 0.5
			continue
		}
		c := 0
		for i := 0; i < draftfeat.NColours; i++ {
			if mask&draftfeat.PairMask[a]&(1<<i) != 0 {
				c++
			}
		}
		out[a] = float64(c) / float64(n)
	}
	return
}

func direction(round int) int {
	if round%2 == 0 {
		return -1
	}
	return 1
}

func mod(x, n int) int { return ((x % n) + n) % n }

func hasID(cs []limited.CardRef, id string) bool {
	for _, c := range cs {
		if c.ID == id {
			return true
		}
	}
	return false
}

func refByID(cs []limited.CardRef, id string) limited.CardRef {
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	return limited.CardRef{}
}

func sumW(w []float64) float64 {
	s := 0.0
	for _, x := range w {
		s += x
	}
	return s
}

// drawIdx draws an index with probability proportional to w (uniform when
// every weight is zero).
func drawIdx(w []float64, r *rand.Rand) int {
	tot := sumW(w)
	if tot <= 0 || math.IsNaN(tot) {
		return r.IntN(len(w))
	}
	x := r.Float64() * tot
	for i, v := range w {
		x -= v
		if x < 0 {
			return i
		}
	}
	return len(w) - 1
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
