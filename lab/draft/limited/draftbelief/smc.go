package draftbelief

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/adams-shaun/gorge/lab/draft/limited"
)

// The sequential sampler. The pool model makes each seat's pick depend on
// its pool so far, which a per-set posterior cannot carry; a particle
// filter can. Every particle is one full history of the other seats'
// picks. Events (each removed set, each unseen pick) are processed in pick
// time; a set's order is drawn from the pool model under the particle's
// pools at that time and the particle is weighted by the set's likelihood
// (the sum over its orders); an unseen pick is drawn from the filler law
// under the seat's pool. Particles are resampled when the effective sample
// size halves. The surviving particles are draws from the joint posterior
// of every seat's pool given the observer's log.

// SMCOptions tunes SampleSMC.
type SMCOptions struct {
	Particles   int
	Runs        int     // independent filters, combined by evidence (0 = 1)
	Beta, Kappa float64 // the pool model, in table z units
}

// DefaultSMC is the setting the report measured.
var DefaultSMC = SMCOptions{Particles: 256, Runs: 8, Beta: 8, Kappa: 1.5}

type smcEvent struct {
	time      int
	set, slot int // one of them >= 0
}

type particle struct {
	typ  [MaxOpp]int8
	cw   [MaxOpp][5]float64
	tot  [MaxOpp]float64
	perm []int8
	seq  [][]int // large sets only
	fill []int32 // per slot: table row (-1 unset)
}

func (f *Flow) events() []smcEvent {
	var ev []smcEvent
	for s := range f.sets {
		ev = append(ev, smcEvent{time: f.sets[s].time[0], set: s, slot: -1})
	}
	for i := range f.slots {
		ev = append(ev, smcEvent{time: f.slots[i].time, set: -1, slot: i})
	}
	sort.SliceStable(ev, func(i, j int) bool { return ev[i].time < ev[j].time })
	return ev
}

func (p *particle) add(o int, mask uint8) {
	p.tot[o-1]++
	for c := 0; c < 5; c++ {
		if mask&(1<<c) != 0 {
			p.cw[o-1][c]++
		}
	}
}

func (p *particle) clone() *particle {
	q := *p
	q.perm = append([]int8(nil), p.perm...)
	q.fill = append([]int32(nil), p.fill...)
	q.seq = make([][]int, len(p.seq))
	for i, s := range p.seq {
		q.seq[i] = append([]int(nil), s...)
	}
	return &q
}

// SampleSMC draws k pools for offset target from the pool-model posterior
// (Sample's constraints apply).
//
// With Runs > 1 it runs that many independent filters of Particles each
// and draws from their union, each run weighted by its evidence estimate:
// one filter can lock into a wrong labelling of the seats early (the first
// picks are nearly symmetric between seats), and the evidence of a run that
// found the right one dominates.
func (f *Flow) SampleSMC(target, k int, c *Constraints, opt SMCOptions, r *rand.Rand) [][]limited.CardRef {
	g := f.prepare()
	cl := f.claim(c)
	runs := max(opt.Runs, 1)
	var ps []*particle
	var lw []float64
	for run := 0; run < runs; run++ {
		rp, rlw, lz := f.smcRun(g, cl, opt, r)
		for i := range rlw {
			rlw[i] += lz
		}
		ps = append(ps, rp...)
		lw = append(lw, rlw...)
	}
	np := len(ps)
	mx := math.Inf(-1)
	for _, x := range lw {
		mx = math.Max(mx, x)
	}
	ww := make([]float64, np)
	for i, x := range lw {
		ww[i] = math.Exp(x - mx)
	}
	return f.smcPools(g, cl, ps, ww, target, k, r)
}

// smcRun is one filter: its particles, their final log weights relative
// to the run, and the run's log evidence estimate.
func (f *Flow) smcRun(g *gibbs, cl claims, opt SMCOptions, r *rand.Rand) ([]*particle, []float64, float64) {
	logZ := 0.0
	np := max(opt.Particles, 1)
	ps := make([]*particle, np)
	for i := range ps {
		ps[i] = &particle{perm: make([]int8, len(f.sets)), seq: make([][]int, len(f.sets)), fill: make([]int32, len(f.slots))}
		for j := range ps[i].fill {
			ps[i].fill[j] = -1
		}
		for o := range ps[i].typ {
			ps[i].typ[o] = int8(r.IntN(len(f.types)))
		}
	}
	lw := make([]float64, np)
	ev := f.events()
	scale := opt.Kappa / math.Max(f.t.Model.Kappa, 1e-9)
	var w []float64
	var sc [MaxOpp][]float64
	for _, e := range ev {
		if e.set >= 0 {
			s := e.set
			rs := &f.sets[s]
			gs := &g.sets[s]
			for pi, p := range ps {
				// Scores of every s0 card for each picking position.
				nr := len(rs.refs)
				for m := 0; m < nr; m++ {
					o := rs.offs[m]
					if cap(sc[m]) < len(rs.s0) {
						sc[m] = make([]float64, len(rs.s0))
					}
					sc[m] = sc[m][:len(rs.s0)]
					for i, ci := range rs.s0 {
						sc[m][i] = math.Exp(opt.Beta * (f.tval(p.typ[o-1], ci) + rs.kap[m]*scale*poolFit(f.mask(ci), &p.cw[o-1], p.tot[o-1])))
					}
				}
				if len(gs.perms) == 0 {
					// Large set: seat by seat within the set.
					seq := p.seq[s][:0]
					used := make([]bool, len(rs.s0))
					taken := make([]bool, nr)
					lik := 1.0
					for m := 0; m < nr; m++ {
						den := 0.0
						for i := range rs.s0 {
							if !used[i] {
								den += sc[m][i]
							}
						}
						ww := make([]float64, nr)
						for j := 0; j < nr; j++ {
							if !taken[j] {
								if o, ok := cl.inst[[2]int{s, j}]; !ok || o == rs.offs[m] {
									ww[j] = sc[m][rs.rpos[j]] / den
								}
							}
						}
						z := sumW(ww)
						if z <= 0 {
							for j := 0; j < nr; j++ {
								if !taken[j] {
									ww[j] = 1e-12
								}
							}
							z = sumW(ww)
						}
						lik *= z
						j := drawIdx(ww, r)
						taken[j], used[rs.rpos[j]] = true, true
						seq = append(seq, j)
					}
					p.seq[s] = seq
					p.perm[s] = -1
					lw[pi] += math.Log(math.Max(lik, 1e-300))
					for m, j := range seq {
						p.add(rs.offs[m], f.mask(rs.s0[rs.rpos[j]]))
					}
					continue
				}
				w = w[:0]
				for _, perm := range gs.perms {
					x := 1.0
					// Π_m sc[m][c_m] / (Σ_{unused} sc[m])
					var usedMask uint64
					for m, j := range perm {
						if o, ok := cl.inst[[2]int{s, j}]; ok && o != rs.offs[m] {
							x = 0
							break
						}
						den := 0.0
						for i := range rs.s0 {
							if usedMask&(1<<uint(i)) == 0 {
								den += sc[m][i]
							}
						}
						pos := rs.rpos[j]
						x *= sc[m][pos] / den
						usedMask |= 1 << uint(pos)
					}
					w = append(w, x)
				}
				z := sumW(w)
				if z <= 0 {
					for i := range w {
						w[i] = 1
					}
					z = 1e-300 * float64(len(w))
				}
				pidx := drawIdx(w, r)
				p.perm[s] = int8(pidx)
				lw[pi] += math.Log(z)
				for m, j := range gs.perms[pidx] {
					p.add(rs.offs[m], f.mask(rs.s0[rs.rpos[j]]))
				}
			}
		} else {
			i := e.slot
			sl := &f.slots[i]
			gl := &g.slots[i]
			for _, p := range ps {
				if name, ok := cl.slot[i]; ok {
					ci := int32(f.t.Index(name))
					p.fill[i] = ci
					p.add(sl.off, f.mask(ci))
					continue
				}
				o := sl.off
				zv := 0.0
				for _, ci := range sl.vis {
					zv += math.Exp(opt.Beta * (f.tval(p.typ[o-1], ci) + sl.kap*scale*poolFit(f.mask(ci), &p.cw[o-1], p.tot[o-1])))
				}
				if cap(w) < len(gl.cand) {
					w = make([]float64, len(gl.cand))
				}
				w = w[:len(gl.cand)]
				for j, ci := range gl.cand {
					x := math.Exp(opt.Beta * (f.tval(p.typ[o-1], ci) + sl.kap*scale*poolFit(f.mask(ci), &p.cw[o-1], p.tot[o-1])))
					w[j] = f.freq[ci] * x / (x + zv)
				}
				ci := gl.cand[drawIdx(w, r)]
				p.fill[i] = ci
				p.add(o, f.mask(ci))
			}
		}
		// Resample on a halved effective sample size.
		mx := math.Inf(-1)
		for _, x := range lw {
			mx = math.Max(mx, x)
		}
		ww := make([]float64, np)
		s1, s2 := 0.0, 0.0
		for i, x := range lw {
			ww[i] = math.Exp(x - mx)
			s1 += ww[i]
			s2 += ww[i] * ww[i]
		}
		if s1*s1/s2 < float64(np)/2 {
			logZ += mx + math.Log(s1/float64(np))
			idx := systematicIdx(ww, np, r)
			nps := make([]*particle, np)
			for i, j := range idx {
				nps[i] = ps[j].clone()
			}
			ps = nps
			for i := range lw {
				lw[i] = 0
			}
		}
	}
	mx := math.Inf(-1)
	for _, x := range lw {
		mx = math.Max(mx, x)
	}
	s1 := 0.0
	for i, x := range lw {
		lw[i] = x - mx
		s1 += math.Exp(lw[i])
	}
	logZ += mx + math.Log(s1/float64(np))
	for i := range lw {
		lw[i] -= math.Log(s1)
	}
	return ps, lw, logZ
}

// smcPools draws k target pools from weighted particles.
func (f *Flow) smcPools(g *gibbs, cl claims, ps []*particle, ww []float64, target, k int, r *rand.Rand) [][]limited.CardRef {
	out := make([][]limited.CardRef, 0, k)
	for d, pi := range systematicIdx(ww, k, r) {
		p := ps[pi]
		var pool []limited.CardRef
		for s := range f.sets {
			rs := &f.sets[s]
			order := p.seq[s]
			if p.perm[s] >= 0 {
				order = g.sets[s].perms[p.perm[s]]
			}
			for m, j := range order {
				if rs.offs[m] == target {
					pool = append(pool, rs.refs[j])
				}
			}
		}
		for i, sl := range f.slots {
			if sl.off != target || p.fill[i] < 0 {
				continue
			}
			pool = append(pool, f.ref(f.t.Cards[p.fill[i]].Name, d, i, "u"))
		}
		for i, name := range cl.extra[target-1] {
			pool = append(pool, f.ref(name, d, i, "x"))
		}
		out = append(out, pool)
	}
	return out
}

// systematicIdx draws k indices proportional to w (systematic resampling).
func systematicIdx(w []float64, k int, r *rand.Rand) []int {
	tot := sumW(w)
	out := make([]int, 0, k)
	if tot <= 0 || math.IsNaN(tot) {
		for i := 0; i < k; i++ {
			out = append(out, r.IntN(len(w)))
		}
		return out
	}
	step := tot / float64(k)
	u := r.Float64() * step
	acc, j := w[0], 0
	for i := 0; i < k; i++ {
		x := u + float64(i)*step
		for acc <= x && j < len(w)-1 {
			j++
			acc += w[j]
		}
		out = append(out, j)
	}
	return out
}

// tval is card ci's value under value type k.
func (f *Flow) tval(k int8, ci int32) float64 {
	if ci < 0 {
		return f.t.Model.VMissing
	}
	return f.types[k][ci]
}
