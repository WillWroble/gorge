package netsimd_test

import (
	"math"
	"testing"

	"github.com/adams-shaun/gorge/bots/gonet"
	"github.com/adams-shaun/gorge/bots/netsimd"
	"github.com/adams-shaun/gorge/bots/policynet"
	"github.com/adams-shaun/gorge/engine/state"
)

func loadRef(t testing.TB, cards bool) *gonet.Net {
	t.Helper()
	gonet.CacheSize = 0
	g, err := gonet.Load(netPath(t))
	if err != nil {
		t.Fatal(err)
	}
	g.SetCardCaches(cards)
	return g
}

func loadSIMD(t testing.TB, cache int) *netsimd.Net {
	t.Helper()
	g, err := netsimd.Load(netPath(t), cache)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

type parity struct {
	maxAbs, maxRel     float64
	over               int // |rel| > 1e-5 where |ref| > 0.1
	n                  int
	absAt, relAt       string
	maxAbsV, maxRelV   float64
	refAtAbs, gotAtAbs float32
	refAtRel, gotAtRel float32
}

func (p *parity) add(where string, ref, got float32) {
	p.n++
	a := math.Abs(float64(ref - got))
	if a > p.maxAbs {
		p.maxAbs, p.absAt, p.refAtAbs, p.gotAtAbs = a, where, ref, got
	}
	if math.Abs(float64(ref)) > 0.1 {
		r := a / math.Abs(float64(ref))
		if r > p.maxRel {
			p.maxRel, p.relAt, p.refAtRel, p.gotAtRel = r, where, ref, got
		}
		if r > 1e-5 {
			p.over++
		}
	}
}

// The SIMD evaluator agrees with gonet on every recorded decision: value
// logit and every option logit.
func TestParityWithGonet(t *testing.T) {
	recs := corpus(t)
	ref := loadRef(t, false)
	for _, name := range []string{"simd", "scalar"} {
		netsimd.ForceScalar = name == "scalar"
		g := loadSIMD(t, 0)
		var sc netsimd.Scratch
		var pv, ps parity
		for i := range recs {
			r := &recs[i]
			a := make([]float32, len(r.opts))
			b := make([]float32, len(r.opts))
			va := ref.Eval(&r.st, r.opts, a)
			vb := g.Eval(&sc, &r.st, r.opts, b)
			pv.add(itoa(i), va, vb)
			for j := range a {
				ps.add(itoa(i)+"/"+itoa(j), a[j], b[j])
			}
		}
		netsimd.ForceScalar = false
		t.Logf("%s value : n=%d max abs %.3g (ref %v got %v at %s) max rel %.3g (ref %v got %v at %s) over1e-5=%d",
			name, pv.n, pv.maxAbs, pv.refAtAbs, pv.gotAtAbs, pv.absAt, pv.maxRel, pv.refAtRel, pv.gotAtRel, pv.relAt, pv.over)
		t.Logf("%s option: n=%d max abs %.3g (ref %v got %v at %s) max rel %.3g (ref %v got %v at %s) over1e-5=%d",
			name, ps.n, ps.maxAbs, ps.refAtAbs, ps.gotAtAbs, ps.absAt, ps.maxRel, ps.refAtRel, ps.gotAtRel, ps.relAt, ps.over)
		if pv.maxAbs > 1e-3 || ps.maxAbs > 1e-3 {
			t.Errorf("%s: logits differ from gonet by more than 1e-3", name)
		}
	}
}

// The card cache changes nothing: a warm second pass equals the cold first.
func TestCardCacheIsTransparent(t *testing.T) {
	recs := corpus(t)
	cold := loadSIMD(t, 0)
	warm := loadSIMD(t, 1<<14)
	var sc netsimd.Scratch
	for pass := 0; pass < 2; pass++ {
		for i := range recs {
			r := &recs[i]
			a := make([]float32, len(r.opts))
			b := make([]float32, len(r.opts))
			va := cold.Eval(&sc, &r.st, r.opts, a)
			vb := warm.Eval(&sc, &r.st, r.opts, b)
			if math.Float32bits(va) != math.Float32bits(vb) {
				t.Fatalf("pass %d rec %d: value %v vs %v", pass, i, va, vb)
			}
			for j := range a {
				if math.Float32bits(a[j]) != math.Float32bits(b[j]) {
					t.Fatalf("pass %d rec %d opt %d: %v vs %v", pass, i, j, a[j], b[j])
				}
			}
		}
	}
}

// SIMD and the scalar fallback are bit-identical on the corpus.
func TestSIMDMatchesScalarOnCorpus(t *testing.T) {
	if !netsimd.SIMD() {
		t.Skip("no AVX-512")
	}
	recs := corpus(t)
	g := loadSIMD(t, 0)
	var sc netsimd.Scratch
	for i := range recs {
		r := &recs[i]
		a := make([]float32, len(r.opts))
		b := make([]float32, len(r.opts))
		netsimd.ForceScalar = false
		va := g.Eval(&sc, &r.st, r.opts, a)
		netsimd.ForceScalar = true
		vb := g.Eval(&sc, &r.st, r.opts, b)
		netsimd.ForceScalar = false
		if math.Float32bits(va) != math.Float32bits(vb) {
			t.Fatalf("rec %d: value %v vs %v", i, va, vb)
		}
		for j := range a {
			if math.Float32bits(a[j]) != math.Float32bits(b[j]) {
				t.Fatalf("rec %d opt %d: %v vs %v", i, j, a[j], b[j])
			}
		}
	}
}

func TestEvalAllocatesNothing(t *testing.T) {
	recs := corpus(t)
	g := loadSIMD(t, 0)
	var sc netsimd.Scratch
	scores := make([]float32, 64)
	r := &recs[len(recs)/2]
	g.Eval(&sc, &r.st, r.opts, scores)
	if n := testing.AllocsPerRun(100, func() { g.Eval(&sc, &r.st, r.opts, scores) }); n != 0 {
		t.Errorf("Eval allocates %v per call", n)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

// ---- benchmarks ----------------------------------------------------------------

func report(b *testing.B, recs []record) {
	s := stats(recs)
	b.ReportMetric(float64(s.opts)/float64(s.n), "opts/eval")
	b.ReportMetric(float64(s.cards)/float64(s.n), "cards/eval")
}

// (a) gonet's eval with its card caches off: every card recomputed.
func BenchmarkGonetNoCache(b *testing.B) {
	recs := corpus(b)
	g := loadRef(b, false)
	scores := make([]float32, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.Eval(&r.st, r.opts, scores)
	}
	report(b, recs)
}

// (b) gonet's eval as the bench runs it: card caches on and warm.
func BenchmarkGonetCardCache(b *testing.B) {
	recs := corpus(b)
	g := loadRef(b, true)
	scores := make([]float32, 64)
	for i := range recs {
		g.Eval(&recs[i].st, recs[i].opts, scores)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.Eval(&r.st, r.opts, scores)
	}
	report(b, recs)
}

// (c) the SIMD evaluator, every card recomputed.
func BenchmarkSIMDNoCache(b *testing.B) {
	recs := corpus(b)
	g := loadSIMD(b, 0)
	var sc netsimd.Scratch
	scores := make([]float32, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.Eval(&sc, &r.st, r.opts, scores)
	}
	report(b, recs)
}

// (c') the SIMD evaluator with its card cache warm.
func BenchmarkSIMDCardCache(b *testing.B) {
	recs := corpus(b)
	g := loadSIMD(b, 1<<14)
	var sc netsimd.Scratch
	scores := make([]float32, 64)
	for i := range recs {
		g.Eval(&sc, &recs[i].st, recs[i].opts, scores)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.Eval(&sc, &r.st, r.opts, scores)
	}
	report(b, recs)
}

// The same layout through the Go fallback kernels: what the transposed,
// padded layout buys on its own, without AVX-512.
func BenchmarkScalarNoCache(b *testing.B) {
	recs := corpus(b)
	g := loadSIMD(b, 0)
	var sc netsimd.Scratch
	scores := make([]float32, 64)
	netsimd.ForceScalar = true
	defer func() { netsimd.ForceScalar = false }()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.Eval(&sc, &r.st, r.opts, scores)
	}
	report(b, recs)
}

// Value only (a search leaf): no options.
func BenchmarkSIMDValueOnly(b *testing.B) {
	recs := corpus(b)
	g := loadSIMD(b, 0)
	var sc netsimd.Scratch
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.Eval(&sc, &r.st, nil, nil)
	}
	report(b, recs)
}

func BenchmarkGonetValueOnlyNoCache(b *testing.B) {
	recs := corpus(b)
	g := loadRef(b, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.Eval(&r.st, nil, nil)
	}
	report(b, recs)
}

// (d) building the inputs: the state encoding per decision ...
func BenchmarkEncodeState(b *testing.B) {
	recs := corpus(b)
	sb := policynet.AcquireStateBuf()
	defer policynet.ReleaseStateBuf(sb)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		policynet.EncodeStateWithInto(sb, policynet.FeaturesEntity, &r.view, state.PlayerID(r.seat), nil)
	}
	report(b, recs)
}

// ... and the option encoding per decision (all of its options).
func BenchmarkEncodeOptions(b *testing.B) {
	recs := corpus(b)
	dst := make([]policynet.Option, 0, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		dst = policynet.EncodeOptionsWith(policynet.FeaturesEntity, &r.view, state.PlayerID(r.seat), r.kind, r.dopt, dst[:0])
	}
	report(b, recs)
}
