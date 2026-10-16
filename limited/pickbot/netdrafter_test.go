package pickbot

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"testing"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draft"
	"github.com/adams-shaun/gorge/limited/draftfeat"
	"github.com/adams-shaun/gorge/limited/draftnet"
)

func netFixture(t testing.TB) (*draftnet.Net, *draftfeat.SetTable) {
	t.Helper()
	b, err := os.ReadFile("../draftnet/testdata/parity.dpkw")
	if err != nil {
		t.Fatal(err)
	}
	n, err := draftnet.Load(b)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2))
	var rows []draftfeat.Card
	for i := 0; i < 40; i++ {
		c := draftfeat.Card{Name: fmt.Sprintf("N%02d", i), Mask: 1 << (i % 5), MV: uint8(1 + i%5),
			Types: draftfeat.TypeCreature, V: r.NormFloat64()}
		c.Pips[i%5] = 1
		rows = append(rows, c)
	}
	tab, err := draftfeat.NewSetTable("TST", rows, draftfeat.DefaultModel(), []float64{0})
	if err != nil {
		t.Fatal(err)
	}
	return n, tab
}

type recorded struct {
	pack, logits, chosen int
	pickedID             string
}

type sliceRecorder struct{ got []recorded }

func (s *sliceRecorder) Record(v PickView, f *draftfeat.Features, in *draftnet.Inputs, logits []float32, chosen int) {
	s.got = append(s.got, recorded{pack: len(v.Pack), logits: len(logits), chosen: chosen, pickedID: v.Pack[chosen].ID})
}

// runNetPod drafts a whole 4-seat pod with NetDrafters and returns every
// pick in order.
func runNetPod(t *testing.T, d *NetDrafter, tab *draftfeat.SetTable, seed uint64) []string {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 7))
	const n, rounds, size = 4, 3, 14
	packs := make([][][]limited.CardRef, n)
	for s := range packs {
		packs[s] = make([][]limited.CardRef, rounds)
		for rd := range packs[s] {
			for j := 0; j < size; j++ {
				c := tab.Cards[r.IntN(len(tab.Cards))]
				packs[s][rd] = append(packs[s][rd], limited.CardRef{ID: fmt.Sprintf("%d-%d-%d", s, rd, j), Name: c.Name, Set: "TST", Rarity: limited.Common})
			}
		}
	}
	p, err := draft.NewPod(seed, packs)
	if err != nil {
		t.Fatal(err)
	}
	seatRng := rand.New(rand.NewPCG(seed, 11))
	var picks []string
	for progress := true; progress; {
		progress = false
		for s := 0; s < n; s++ {
			v, ok := p.Next(s)
			if !ok {
				continue
			}
			id := d.Pick(v, seatRng)
			if !slices.ContainsFunc(v.Pack, func(c limited.CardRef) bool { return c.ID == id }) {
				t.Fatalf("seat %d picked %q, not in its pack", s, id)
			}
			if err := p.Pick(s, v.PickNo, id); err != nil {
				t.Fatal(err)
			}
			picks = append(picks, id)
			progress = true
		}
	}
	if len(picks) != n*rounds*size {
		t.Fatalf("%d picks, want %d", len(picks), n*rounds*size)
	}
	return picks
}

func TestNetDrafterDraftsLegallyAndDeterministically(t *testing.T) {
	net, tab := netFixture(t)
	for _, temp := range []float64{0, 1} {
		rec := &sliceRecorder{}
		d := &NetDrafter{Net: net, Table: tab, Temperature: temp, Recorder: rec}
		a := runNetPod(t, d, tab, 5)
		b := runNetPod(t, &NetDrafter{Net: net, Table: tab, Temperature: temp}, tab, 5)
		if !slices.Equal(a, b) {
			t.Fatalf("T=%v: the same seed drafted differently", temp)
		}
		if len(rec.got) != len(a) {
			t.Fatalf("recorder saw %d picks, want %d", len(rec.got), len(a))
		}
		for i, g := range rec.got {
			if g.logits != g.pack || g.pickedID != a[i] {
				t.Fatalf("T=%v pick %d: recorded %+v, picked %s", temp, i, g, a[i])
			}
		}
	}
	var none NetDrafter
	if got := none.Pick(PickView{}, nil); got != "" {
		t.Errorf("empty pack picked %q", got)
	}
}

func TestNetDrafterArgmaxMatchesLogits(t *testing.T) {
	if got := argmaxLogits([]float32{0.1, 2, -1, 0.5}, nil); got != 1 {
		t.Errorf("argmax %d", got)
	}
	r := rand.New(rand.NewPCG(1, 1))
	seen := map[int]bool{}
	for i := 0; i < 64; i++ {
		seen[argmaxLogits([]float32{3, 1, 3, 3}, r)] = true
	}
	if len(seen) != 3 || seen[1] {
		t.Errorf("tie-break reached %v", seen)
	}
	// Temperature sampling at a tiny T is argmax.
	if got := sampleLogits([]float32{0, 5, 1}, 1e-3, r); got != 1 {
		t.Errorf("cold sample %d", got)
	}
}

// TestSampleLogitsMatchesSoftmax: over many draws the empirical pick
// frequencies match softmax(l/T), and SoftmaxLogProb is that softmax.
func TestSampleLogitsMatchesSoftmax(t *testing.T) {
	l := []float32{0.3, 1.2, -0.5, 1.0, 0.0}
	r := rand.New(rand.NewPCG(11, 3))
	for _, temp := range []float64{0.5, 1, 2} {
		const n = 200000
		var counts [5]int
		for i := 0; i < n; i++ {
			counts[sampleLogits(l, temp, r)]++
		}
		var psum float64
		for i := range l {
			p := math.Exp(SoftmaxLogProb(l, temp, i))
			psum += p
			got := float64(counts[i]) / n
			// 5 sigma of a binomial proportion.
			if tol := 5 * math.Sqrt(p*(1-p)/n); math.Abs(got-p) > tol {
				t.Errorf("T=%v card %d: frequency %.4f, softmax %.4f (tol %.4f)", temp, i, got, p, tol)
			}
		}
		if math.Abs(psum-1) > 1e-9 {
			t.Errorf("T=%v: probabilities sum to %v", temp, psum)
		}
	}
}

// TestNetDrafterColdTemperatureIsArgmax: as T -> 0 a sampling NetDrafter
// drafts exactly the argmax pod.
func TestNetDrafterColdTemperatureIsArgmax(t *testing.T) {
	net, tab := netFixture(t)
	a := runNetPod(t, &NetDrafter{Net: net, Table: tab}, tab, 7)
	b := runNetPod(t, &NetDrafter{Net: net, Table: tab, Temperature: 1e-6}, tab, 7)
	if !slices.Equal(a, b) {
		t.Fatal("T=1e-6 drafted differently from argmax")
	}
	c := runNetPod(t, &NetDrafter{Net: net, Table: tab, Temperature: 1}, tab, 7)
	if slices.Equal(a, c) {
		t.Log("T=1 happened to match argmax on this pod")
	}
}
