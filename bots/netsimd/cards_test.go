package netsimd_test

import (
	"math"
	"sync"
	"testing"

	"github.com/adams-shaun/gorge/bots/netsimd"
	"github.com/adams-shaun/gorge/bots/policynet"
)

// Every precomputed path -- vectors, identity halves, raw features and
// rows, and a mix -- gives Eval's logits bit for bit, through the AVX-512
// kernels and the scalar fallback.
func TestPrecomputedCardsMatchEval(t *testing.T) {
	recs := corpus(t)
	for _, scalar := range []bool{false, true} {
		netsimd.ForceScalar = scalar
		g := loadSIMD(t, 0)
		var sc, sc2 netsimd.Scratch
		w := g.CardWidth()
		for i := range recs {
			r := &recs[i]
			want := make([]float32, len(r.opts))
			wv := g.Eval(&sc, &r.st, r.opts, want)
			n := len(r.st.Cards)
			vecs := make([]float32, n*w)
			idents := make([]float32, n*w)
			for k := range r.st.Cards {
				c := &r.st.Cards[k]
				g.CardIdentity(&sc2, c.Rows, idents[k*w:(k+1)*w])
				g.CardVector(&sc2, c.Raw, idents[k*w:(k+1)*w], vecs[k*w:(k+1)*w])
				if got := sc.CardVec(k, w); !sameBits(got, vecs[k*w:(k+1)*w]) {
					t.Fatalf("scalar=%v rec %d card %d: CardVector differs from Eval's vector", scalar, i, k)
				}
			}
			for mode := 0; mode < 4; mode++ {
				in := make([]netsimd.CardIn, n)
				for k := range r.st.Cards {
					c := &r.st.Cards[k]
					in[k] = netsimd.CardIn{Group: c.Group}
					switch (mode + k) % 3 {
					case 0:
						in[k].Vec = vecs[k*w : (k+1)*w]
					case 1:
						in[k].Raw, in[k].Ident = c.Raw, idents[k*w:(k+1)*w]
					default:
						in[k].Raw, in[k].Rows = c.Raw, c.Rows
					}
					if mode == 3 {
						in[k] = netsimd.CardIn{Group: c.Group, Vec: vecs[k*w : (k+1)*w]}
					}
				}
				got := make([]float32, len(r.opts))
				gv := g.EvalCards(&sc2, in, r.st.Dense, r.st.Sparse, r.opts, got)
				if math.Float32bits(gv) != math.Float32bits(wv) {
					t.Fatalf("scalar=%v rec %d mode %d: value %v, Eval %v", scalar, i, mode, gv, wv)
				}
				if !sameBits(got, want) {
					t.Fatalf("scalar=%v rec %d mode %d: option logits %v, Eval %v", scalar, i, mode, got, want)
				}
			}
		}
	}
	netsimd.ForceScalar = false
}

func sameBits(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			return false
		}
	}
	return true
}

// The store: one writer per slot wins, readers see a slot only once it is
// whole, and nothing allocates after construction.
func TestCardStore(t *testing.T) {
	g := loadSIMD(t, 0)
	w := g.CardWidth()
	const slots = 256
	cs := netsimd.NewCardStore(g, slots)
	if cs.Get(0) != nil || cs.Get(-1) != nil || cs.Get(slots) != nil {
		t.Fatal("empty or out-of-range slot reads non-nil")
	}
	vec := func(slot int) []float32 {
		v := make([]float32, w)
		for i := range v {
			v[i] = float32(slot*w + i)
		}
		return v
	}
	var wg sync.WaitGroup
	for gr := 0; gr < 8; gr++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := 0; s < slots; s++ {
				cs.Put(s, vec(s))
				if v := cs.Get(s); v != nil && !sameBits(v, vec(s)) {
					t.Errorf("slot %d read torn", s)
				}
			}
		}()
	}
	wg.Wait()
	for s := 0; s < slots; s++ {
		if !sameBits(cs.Get(s), vec(s)) {
			t.Fatalf("slot %d: wrong vector", s)
		}
	}
	if cs.Put(3, vec(4)) {
		t.Fatal("a second Put wrote a filled slot")
	}
	v := vec(7)
	if a := testing.AllocsPerRun(100, func() { cs.Get(7); cs.Put(7, v) }); a != 0 {
		t.Fatalf("Get/Put allocate %v", a)
	}
	cs.Reset()
	if cs.Get(7) != nil {
		t.Fatal("Reset left a slot")
	}
}

// EvalCards with every vector precomputed allocates nothing.
func TestEvalCardsAllocatesNothing(t *testing.T) {
	recs := corpus(t)
	g := loadSIMD(t, 0)
	var sc netsimd.Scratch
	w := g.CardWidth()
	r := &recs[len(recs)/2]
	in := make([]netsimd.CardIn, len(r.st.Cards))
	for k := range r.st.Cards {
		c := &r.st.Cards[k]
		id := make([]float32, w)
		g.CardIdentity(&sc, c.Rows, id)
		in[k] = netsimd.CardIn{Group: c.Group, Raw: c.Raw, Ident: id}
	}
	scores := make([]float32, 64)
	g.EvalCards(&sc, in, r.st.Dense, r.st.Sparse, r.opts, scores)
	if a := testing.AllocsPerRun(100, func() { g.EvalCards(&sc, in, r.st.Dense, r.st.Sparse, r.opts, scores) }); a != 0 {
		t.Fatalf("EvalCards allocates %v per call", a)
	}
}

// Precomputed card vectors against Eval (every card computed).
func BenchmarkSIMDPrecomputedCards(b *testing.B) {
	recs := corpus(b)
	g := loadSIMD(b, 0)
	var sc netsimd.Scratch
	w := g.CardWidth()
	ins := make([][]netsimd.CardIn, len(recs))
	for i := range recs {
		r := &recs[i]
		for k := range r.st.Cards {
			c := &r.st.Cards[k]
			id := make([]float32, w)
			v := make([]float32, w)
			g.CardIdentity(&sc, c.Rows, id)
			g.CardVector(&sc, c.Raw, id, v)
			ins[i] = append(ins[i], netsimd.CardIn{Group: c.Group, Vec: v})
		}
	}
	scores := make([]float32, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &recs[i%len(recs)]
		g.EvalCards(&sc, ins[i%len(recs)], r.st.Dense, r.st.Sparse, r.opts, scores)
	}
	report(b, recs)
}

// EvalBag over the bag summed in one part is Eval bit for bit; summed in
// parts (here: halves, added) it agrees to float rounding.
func TestEvalBagMatchesEval(t *testing.T) {
	recs := corpus(t)
	g := loadSIMD(t, 0)
	var sc netsimd.Scratch
	w := g.BagWidth()
	var maxd float64
	for i := range recs {
		r := &recs[i]
		want := make([]float32, len(r.opts))
		wv := g.Eval(&sc, &r.st, r.opts, want)
		in := make([]netsimd.CardIn, len(r.st.Cards))
		for k := range r.st.Cards {
			c := &r.st.Cards[k]
			in[k] = netsimd.CardIn{Group: c.Group, Raw: c.Raw, Rows: c.Rows}
		}
		one := make([]float32, w)
		g.BagAdd(one, r.st.Sparse)
		got := make([]float32, len(r.opts))
		gv := g.EvalBag(&sc, in, r.st.Dense, one, r.opts, got)
		if math.Float32bits(gv) != math.Float32bits(wv) || !sameBits(got, want) {
			t.Fatalf("rec %d: one-part bag differs from Eval", i)
		}
		h := len(r.st.Sparse) / 2
		a, b := make([]float32, w), make([]float32, w)
		g.BagAdd(a, r.st.Sparse[:h])
		g.BagAdd(b, r.st.Sparse[h:])
		for j := range a {
			a[j] += b[j]
		}
		gv = g.EvalBag(&sc, in, r.st.Dense, a, r.opts, got)
		maxd = math.Max(maxd, math.Abs(float64(gv-wv)))
		for j := range got {
			maxd = math.Max(maxd, math.Abs(float64(got[j]-want[j])))
		}
	}
	t.Logf("two-part bag: max |logit - Eval| = %.3g", maxd)
	if maxd > 1e-4 {
		t.Fatalf("two-part bag logits differ by %v", maxd)
	}
}

// SumRows over a caller's table with value-1 rows is the plain sum, in
// order, through the AVX-512 kernel and the scalar fallback.
func TestSumRows(t *testing.T) {
	for _, scalar := range []bool{false, true} {
		netsimd.ForceScalar = scalar
		const w, n = 32, 50
		tab := make([]float32, n*w)
		for i := range tab {
			tab[i] = float32(i%97)*0.37 - 11
		}
		rows := []policynet.Feature{{Row: 3, Value: 1}, {Row: 49, Value: 1}, {Row: 3, Value: 1}, {Row: 7, Value: 1}}
		got := make([]float32, w)
		got[5] = 2
		netsimd.SumRows(got, tab, w, rows)
		for j := 0; j < w; j++ {
			want := float32(0)
			if j == 5 {
				want = 2
			}
			for _, r := range rows {
				want += tab[int(r.Row)*w+j]
			}
			if math.Float32bits(got[j]) != math.Float32bits(want) {
				t.Fatalf("scalar=%v lane %d: %v, want %v", scalar, j, got[j], want)
			}
		}
	}
	netsimd.ForceScalar = false
}
