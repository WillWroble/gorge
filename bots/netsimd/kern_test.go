package netsimd

import (
	"math"
	"math/rand/v2"
	"testing"
	"unsafe"

	"github.com/adams-shaun/gorge/bots/policynet"
)

// The bag kernels walk []policynet.Feature in place as []feat.
func TestFeatureLayout(t *testing.T) {
	var f policynet.Feature
	if unsafe.Sizeof(f) != unsafe.Sizeof(feat{}) || unsafe.Offsetof(f.Row) != 0 || unsafe.Offsetof(f.Value) != 4 || unsafe.Sizeof(f) != 8 {
		t.Fatalf("policynet.Feature layout changed: size %d, Row@%d Value@%d", unsafe.Sizeof(f), unsafe.Offsetof(f.Row), unsafe.Offsetof(f.Value))
	}
}

func randF(r *rand.Rand, n int) []float32 {
	s := aligned(n)
	for i := range s {
		s[i] = float32(r.NormFloat64())
	}
	return s
}

func same(a, b []float32) (int, float32) {
	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			return i, a[i] - b[i]
		}
	}
	return -1, 0
}

// Every assembly kernel is bit-identical to its scalar twin.
func TestKernelsMatchScalar(t *testing.T) {
	if !hasAVX512 {
		t.Skip("no AVX-512 on this CPU")
	}
	r := rand.New(rand.NewPCG(1, 2))
	for _, c := range []struct{ n, outPad int }{{64, 64}, {68, 64}, {24, 64}, {32, 64}, {128, 64}, {256, 64}, {64, 128}, {68, 32}, {32, 32}, {1, 64}, {3, 96}, {0, 64}} {
		for _, relu := range []bool{false, true} {
			wt := randF(r, max(c.n, 1)*c.outPad)
			x := randF(r, max(c.n, 1))
			acc := randF(r, c.outPad)
			ya, ys := aligned(c.outPad), aligned(c.outPad)
			ForceScalar = false
			gemvT(&wt[0], &x[0], &acc[0], &ya[0], c.n, c.outPad, relu)
			ForceScalar = true
			gemvT(&wt[0], &x[0], &acc[0], &ys[0], c.n, c.outPad, relu)
			ForceScalar = false
			if i, d := same(ya, ys); i >= 0 {
				t.Errorf("gemvT n=%d outPad=%d relu=%v: lane %d differs by %g (%v vs %v)", c.n, c.outPad, relu, i, d, ya[i], ys[i])
			}
		}
	}
	for _, width := range []int{32, 64} {
		table := randF(r, 300*width)
		feats := make([]feat, 20)
		for i := range feats {
			feats[i] = feat{Row: uint16(r.IntN(320)), Value: float32(r.NormFloat64())}
		}
		ya, ys := randF(r, width), aligned(width)
		copy(ys, ya)
		ForceScalar = false
		bag(&ya[0], &table[0], &feats[0], len(feats), 300, width)
		ForceScalar = true
		bag(&ys[0], &table[0], &feats[0], len(feats), 300, width)
		ForceScalar = false
		if i, d := same(ya, ys); i >= 0 {
			t.Errorf("bag width=%d: lane %d differs by %g", width, i, d)
		}
	}
	{
		x, w, b := randF(r, 64), randF(r, 64), randF(r, 64)
		ya, ys := aligned(64), aligned(64)
		ForceScalar = false
		ln64(&ya[0], &x[0], &w[0], &b[0], 1e-5)
		ForceScalar = true
		ln64(&ys[0], &x[0], &w[0], &b[0], 1e-5)
		ForceScalar = false
		if i, d := same(ya, ys); i >= 0 {
			t.Errorf("ln64: lane %d differs by %g (%v vs %v)", i, d, ya[i], ys[i])
		}
		// in place
		copy(ya, x)
		ln64(&ya[0], &ya[0], &w[0], &b[0], 1e-5)
		if i, d := same(ya, ys); i >= 0 {
			t.Errorf("ln64 in place: lane %d differs by %g", i, d)
		}
	}
	{
		a, b := randF(r, 64), randF(r, 64)
		ForceScalar = false
		da := dot64(&a[0], &b[0])
		ForceScalar = true
		ds := dot64(&a[0], &b[0])
		ForceScalar = false
		if math.Float32bits(da) != math.Float32bits(ds) {
			t.Errorf("dot64: %v vs %v", da, ds)
		}
	}
	{
		x := randF(r, 64)
		ya, ys := randF(r, 64), aligned(64)
		copy(ys, ya)
		ForceScalar = false
		addv64(&ya[0], &x[0])
		ForceScalar = true
		addv64(&ys[0], &x[0])
		ForceScalar = false
		if i, d := same(ya, ys); i >= 0 {
			t.Errorf("addv64: lane %d differs by %g", i, d)
		}
	}
	{
		c := randF(r, 32)
		sa, ma := randF(r, 32), randF(r, 32)
		ss, ms := aligned(32), aligned(32)
		copy(ss, sa)
		copy(ms, ma)
		ForceScalar = false
		poolAcc(&sa[0], &ma[0], &c[0])
		ForceScalar = true
		poolAcc(&ss[0], &ms[0], &c[0])
		ForceScalar = false
		if i, d := same(sa, ss); i >= 0 {
			t.Errorf("poolAcc sum: lane %d differs by %g", i, d)
		}
		if i, d := same(ma, ms); i >= 0 {
			t.Errorf("poolAcc max: lane %d differs by %g", i, d)
		}
	}
}

func TestAlignedIsAligned(t *testing.T) {
	for _, n := range []int{1, 7, 64, 65, 1000} {
		s := aligned(n)
		if len(s) != n || uintptr(unsafe.Pointer(&s[0]))&(align-1) != 0 {
			t.Errorf("aligned(%d): len %d, address %#x", n, len(s), uintptr(unsafe.Pointer(&s[0])))
		}
	}
}
