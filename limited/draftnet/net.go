package draftnet

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// Architecture (draft-RL spec §5): pre-LN transformer, Layers layers, width
// D, Heads heads, feed-forward FF, tanh-GELU, LayerNorm eps LNEps. A card
// token is a CardEmb-wide learned per-set embedding concatenated with a
// CardEmb-wide linear map of the card's facts.
const (
	D       = 64
	Heads   = 4
	HeadDim = D / Heads
	FF      = 128
	Layers  = 2
	CardEmb = 32
	LNEps   = 1e-5
)

// Token types, in token order: CTX, then SEAT, POOL, CAND.
const (
	TypeCtx = iota
	TypeSeat
	TypePool
	TypeCand
	NTypes
)

// shape is one policy tensor's expected shape; a -1 dim is free (the card
// embedding's row count is the set table's size).
type shape struct {
	name     string
	dims     []int
	optional bool
}

// policyTensors is the complete policy graph. Load refuses any other name.
var policyTensors = func() []shape {
	s := []shape{
		{"type.emb", []int{NTypes, D}, false},
		{"card.emb", []int{-1, CardEmb}, false},
		{"card.facts.w", []int{FactDim, CardEmb}, false},
		{"card.facts.b", []int{CardEmb}, false},
		{"pool.pick", []int{MaxPool, D}, false},
		{"cand.w", []int{CandDim, D}, false},
		{"cand.b", []int{D}, false},
		{"seat.off", []int{MaxOthers, D}, false},
		{"seat.w", []int{SeatDim, D}, false},
		{"seat.b", []int{D}, false},
		{"ctx.w", []int{CtxDim, D}, false},
		{"ctx.b", []int{D}, false},
	}
	for l := 0; l < Layers; l++ {
		p := fmt.Sprintf("l%d.", l)
		s = append(s,
			shape{p + "ln1.g", []int{D}, false}, shape{p + "ln1.b", []int{D}, false},
			shape{p + "qkv.w", []int{D, 3 * D}, false}, shape{p + "qkv.b", []int{3 * D}, false},
			shape{p + "out.w", []int{D, D}, false}, shape{p + "out.b", []int{D}, false},
			shape{p + "ln2.g", []int{D}, false}, shape{p + "ln2.b", []int{D}, false},
			shape{p + "ff1.w", []int{D, FF}, false}, shape{p + "ff1.b", []int{FF}, false},
			shape{p + "ff2.w", []int{FF, D}, false}, shape{p + "ff2.b", []int{D}, false},
		)
	}
	return append(s,
		shape{"lnf.g", []int{D}, false}, shape{"lnf.b", []int{D}, false},
		shape{"head.pol.w", []int{D}, false}, shape{"head.pol.b", []int{1}, false},
		// Auxiliary heads (seat pair, diagnostic value): part of the policy
		// graph the trainer fits, never read by Forward.
		shape{"head.seat.w", []int{D, 10}, true}, shape{"head.seat.b", []int{10}, true},
		shape{"head.val.w", []int{D}, true}, shape{"head.val.b", []int{1}, true},
	)
}()

type layer struct {
	ln1g, ln1b, qkvW, qkvB, outW, outB []float32
	ln2g, ln2b, ff1W, ff1B, ff2W, ff2B []float32
}

// Net is a loaded policy. It is immutable and safe to share; each caller
// runs it through its own Runner.
type Net struct {
	Meta []byte

	typeEmb, cardEmb, factsW, factsB, poolPick []float32
	candW, candB, seatOff, seatW, seatB        []float32
	ctxW, ctxB, lnfG, lnfB, polW, polB         []float32
	cards                                      int
	layers                                     [Layers]layer
}

// Cards is the card embedding's row count.
func (n *Net) Cards() int { return n.cards }

// Load decodes a .dpkw policy. It refuses a file whose tensors are not
// exactly the policy graph: an unknown name, a wrong shape, a missing
// required tensor, and above all any critic.* tensor -- the critic is
// training-only and omniscient (spec §2.2), and must never reach play.
func Load(b []byte) (*Net, error) {
	f, err := Decode(b)
	if err != nil {
		return nil, err
	}
	got := make(map[string][]float32, len(f.Tensors)) // load-time only
	for _, t := range f.Tensors {
		if strings.HasPrefix(t.Name, "critic.") || t.Name == "critic" {
			return nil, fmt.Errorf("draftnet: weights carry critic tensor %q; the critic is training-only (spec §2.2)", t.Name)
		}
		var spec *shape
		for i := range policyTensors {
			if policyTensors[i].name == t.Name {
				spec = &policyTensors[i]
			}
		}
		if spec == nil {
			return nil, fmt.Errorf("draftnet: unknown tensor %q", t.Name)
		}
		if _, dup := got[t.Name]; dup {
			return nil, fmt.Errorf("draftnet: duplicate tensor %q", t.Name)
		}
		if len(t.Shape) != len(spec.dims) {
			return nil, fmt.Errorf("draftnet: tensor %q has shape %v, want %v", t.Name, t.Shape, spec.dims)
		}
		for i, d := range spec.dims {
			if d >= 0 && t.Shape[i] != d {
				return nil, fmt.Errorf("draftnet: tensor %q has shape %v, want %v", t.Name, t.Shape, spec.dims)
			}
		}
		for _, x := range t.Data {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				return nil, fmt.Errorf("draftnet: tensor %q holds a non-finite value", t.Name)
			}
		}
		got[t.Name] = t.Data
	}
	for _, s := range policyTensors {
		if _, ok := got[s.name]; !ok && !s.optional {
			return nil, fmt.Errorf("draftnet: missing tensor %q", s.name)
		}
	}
	n := &Net{Meta: f.Meta}
	n.typeEmb, n.cardEmb = got["type.emb"], got["card.emb"]
	n.cards = len(n.cardEmb) / CardEmb
	n.factsW, n.factsB, n.poolPick = got["card.facts.w"], got["card.facts.b"], got["pool.pick"]
	n.candW, n.candB = got["cand.w"], got["cand.b"]
	n.seatOff, n.seatW, n.seatB = got["seat.off"], got["seat.w"], got["seat.b"]
	n.ctxW, n.ctxB = got["ctx.w"], got["ctx.b"]
	n.lnfG, n.lnfB, n.polW, n.polB = got["lnf.g"], got["lnf.b"], got["head.pol.w"], got["head.pol.b"]
	for l := range n.layers {
		p := fmt.Sprintf("l%d.", l)
		n.layers[l] = layer{
			got[p+"ln1.g"], got[p+"ln1.b"], got[p+"qkv.w"], got[p+"qkv.b"], got[p+"out.w"], got[p+"out.b"],
			got[p+"ln2.g"], got[p+"ln2.b"], got[p+"ff1.w"], got[p+"ff1.b"], got[p+"ff2.w"], got[p+"ff2.b"],
		}
	}
	return n, nil
}

// ParamCount is the number of float parameters in the policy graph the
// Forward pass reads, split into the card embedding and everything else.
func (n *Net) ParamCount() (emb, rest int) {
	emb = len(n.cardEmb)
	for _, s := range [][]float32{n.typeEmb, n.factsW, n.factsB, n.poolPick, n.candW, n.candB,
		n.seatOff, n.seatW, n.seatB, n.ctxW, n.ctxB, n.lnfG, n.lnfB, n.polW, n.polB} {
		rest += len(s)
	}
	for _, l := range n.layers {
		for _, s := range [][]float32{l.ln1g, l.ln1b, l.qkvW, l.qkvB, l.outW, l.outB, l.ln2g, l.ln2b, l.ff1W, l.ff1B, l.ff2W, l.ff2B} {
			rest += len(s)
		}
	}
	return emb, rest
}

// Runner is Forward's reusable scratch. Its zero value is ready; after the
// first call at a given token count, Forward allocates nothing.
type Runner struct {
	x, h, qkv, att, ff []float32
	score              []float32
	logits             []float32
}

func grow(s []float32, n int) []float32 {
	if cap(s) < n {
		return make([]float32, n)
	}
	return s[:n]
}

// ErrNoCandidates is Forward's error for an input with no candidate token.
var ErrNoCandidates = errors.New("draftnet: no candidates")

// Forward scores in's candidates: logits[i] is Cand[i]'s policy logit. The
// returned slice is the Runner's and is overwritten by the next call.
func (r *Runner) Forward(n *Net, in *Inputs) ([]float32, error) {
	nc := len(in.Cand)
	if nc == 0 {
		return nil, ErrNoCandidates
	}
	if in.NSeat < 0 || in.NSeat > MaxOthers || len(in.Pool) > MaxPool || nc > MaxCand {
		return nil, errors.New("draftnet: input exceeds the token limits")
	}
	T := 1 + in.NSeat + len(in.Pool) + nc
	r.x = grow(r.x, T*D)
	r.h = grow(r.h, T*D)
	r.qkv = grow(r.qkv, T*3*D)
	r.att = grow(r.att, T*D)
	r.ff = grow(r.ff, T*FF)
	r.score = grow(r.score, T)
	r.logits = grow(r.logits, nc)
	x := r.x

	// Embeddings.
	t := 0
	linear(x[:D], in.Ctx[:], n.ctxW, n.ctxB, D)
	addTo(x[:D], n.typeEmb[TypeCtx*D:(TypeCtx+1)*D])
	t++
	for j := 0; j < in.NSeat; j++ {
		row := x[t*D : (t+1)*D]
		linear(row, in.Seat[j][:], n.seatW, n.seatB, D)
		off := in.SeatOff[j]
		if off < 0 || off >= MaxOthers {
			return nil, errors.New("draftnet: seat offset out of range")
		}
		addTo(row, n.seatOff[off*D:(off+1)*D])
		addTo(row, n.typeEmb[TypeSeat*D:(TypeSeat+1)*D])
		t++
	}
	for j := range in.Pool {
		p := &in.Pool[j]
		row := x[t*D : (t+1)*D]
		n.cardToken(row, &p.Card)
		k := int(p.Pick)
		if k < 0 {
			k = 0
		} else if k >= MaxPool {
			k = MaxPool - 1
		}
		addTo(row, n.poolPick[k*D:(k+1)*D])
		addTo(row, n.typeEmb[TypePool*D:(TypePool+1)*D])
		t++
	}
	c0 := t
	for j := range in.Cand {
		c := &in.Cand[j]
		row := x[t*D : (t+1)*D]
		n.cardToken(row, &c.Card)
		for o := 0; o < D; o++ {
			s := n.candB[o]
			for i := 0; i < CandDim; i++ {
				s += c.Feat[i] * n.candW[i*D+o]
			}
			row[o] += s
		}
		addTo(row, n.typeEmb[TypeCand*D:(TypeCand+1)*D])
		t++
	}

	// Transformer blocks.
	h := r.h
	for li := range n.layers {
		l := &n.layers[li]
		for i := 0; i < T; i++ {
			layerNorm(h[i*D:(i+1)*D], x[i*D:(i+1)*D], l.ln1g, l.ln1b)
			linear(r.qkv[i*3*D:(i+1)*3*D], h[i*D:(i+1)*D], l.qkvW, l.qkvB, 3*D)
		}
		r.attention(T)
		for i := 0; i < T; i++ {
			linear(h[i*D:(i+1)*D], r.att[i*D:(i+1)*D], l.outW, l.outB, D)
			addTo(x[i*D:(i+1)*D], h[i*D:(i+1)*D])
		}
		for i := 0; i < T; i++ {
			layerNorm(h[i*D:(i+1)*D], x[i*D:(i+1)*D], l.ln2g, l.ln2b)
			f := r.ff[i*FF : (i+1)*FF]
			linear(f, h[i*D:(i+1)*D], l.ff1W, l.ff1B, FF)
			for k := range f {
				f[k] = gelu(f[k])
			}
			linear(h[i*D:(i+1)*D], f, l.ff2W, l.ff2B, D)
			addTo(x[i*D:(i+1)*D], h[i*D:(i+1)*D])
		}
	}

	// Policy head over the candidate tokens.
	for j := 0; j < nc; j++ {
		i := c0 + j
		layerNorm(h[i*D:(i+1)*D], x[i*D:(i+1)*D], n.lnfG, n.lnfB)
		s := n.polB[0]
		for o := 0; o < D; o++ {
			s += h[i*D+o] * n.polW[o]
		}
		r.logits[j] = s
	}
	return r.logits, nil
}

// cardToken writes emb[c.Emb] ‖ facts·W + b into row; an out-of-range or
// negative Emb (a card the table lacks) has a zero embedding.
func (n *Net) cardToken(row []float32, c *Card) {
	if e := int(c.Emb); e >= 0 && e < n.cards {
		copy(row[:CardEmb], n.cardEmb[e*CardEmb:(e+1)*CardEmb])
	} else {
		clear(row[:CardEmb])
	}
	linear(row[CardEmb:D], c.Facts[:], n.factsW, n.factsB, CardEmb)
}

// attention runs multi-head self-attention over r.qkv into r.att.
func (r *Runner) attention(T int) {
	const scale = 0.25 // 1/sqrt(HeadDim)
	for hd := 0; hd < Heads; hd++ {
		qo, ko, vo := hd*HeadDim, D+hd*HeadDim, 2*D+hd*HeadDim
		for i := 0; i < T; i++ {
			q := r.qkv[i*3*D+qo : i*3*D+qo+HeadDim]
			mx := float32(math.Inf(-1))
			for j := 0; j < T; j++ {
				k := r.qkv[j*3*D+ko : j*3*D+ko+HeadDim]
				var s float32
				for d := 0; d < HeadDim; d++ {
					s += q[d] * k[d]
				}
				s *= scale
				r.score[j] = s
				if s > mx {
					mx = s
				}
			}
			var sum float32
			for j := 0; j < T; j++ {
				e := float32(math.Exp(float64(r.score[j] - mx)))
				r.score[j] = e
				sum += e
			}
			out := r.att[i*D+qo : i*D+qo+HeadDim]
			clear(out)
			for j := 0; j < T; j++ {
				w := r.score[j] / sum
				v := r.qkv[j*3*D+vo : j*3*D+vo+HeadDim]
				for d := 0; d < HeadDim; d++ {
					out[d] += w * v[d]
				}
			}
		}
	}
}

// linear writes out = in·W + b with W row major [len(in), nOut]. Rows are
// accumulated four at a time, always in the same order.
func linear(out, in, w, b []float32, nOut int) {
	out = out[:nOut]
	copy(out, b[:nOut])
	i := 0
	for ; i+4 <= len(in); i += 4 {
		x0, x1, x2, x3 := in[i], in[i+1], in[i+2], in[i+3]
		r0 := w[i*nOut : (i+1)*nOut]
		r1 := w[(i+1)*nOut : (i+2)*nOut]
		r2 := w[(i+2)*nOut : (i+3)*nOut]
		r3 := w[(i+3)*nOut : (i+4)*nOut]
		r0, r1, r2, r3 = r0[:len(out)], r1[:len(out)], r2[:len(out)], r3[:len(out)]
		for o := range out {
			out[o] += x0*r0[o] + x1*r1[o] + x2*r2[o] + x3*r3[o]
		}
	}
	for ; i < len(in); i++ {
		xi := in[i]
		row := w[i*nOut : (i+1)*nOut]
		row = row[:len(out)]
		for o, wv := range row {
			out[o] += xi * wv
		}
	}
}

func addTo(dst, src []float32) {
	for i := range dst {
		dst[i] += src[i]
	}
}

func layerNorm(out, in, g, b []float32) {
	var mean float32
	for _, v := range in {
		mean += v
	}
	mean /= float32(len(in))
	var vr float32
	for _, v := range in {
		d := v - mean
		vr += d * d
	}
	vr /= float32(len(in))
	inv := float32(1 / math.Sqrt(float64(vr)+LNEps))
	for i, v := range in {
		out[i] = (v-mean)*inv*g[i] + b[i]
	}
}

// gelu is the tanh approximation (torch GELU(approximate="tanh")).
func gelu(x float32) float32 {
	const c = 0.7978845608028654 // sqrt(2/pi)
	xf := float64(x)
	return float32(0.5 * xf * (1 + math.Tanh(c*(xf+0.044715*xf*xf*xf))))
}
