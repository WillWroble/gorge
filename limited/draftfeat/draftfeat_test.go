package draftfeat

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draft"
)

var update = flag.Bool("update", false, "rewrite golden files")

const colours = "WUBRG"

// mono is a hand-built row: one colour (or "" for colourless), a value.
func mono(name, col string, v float64, types uint8) Card {
	c := Card{Name: name, V: v, MV: 2, Types: types}
	if col != "" {
		i := strings.Index(colours, col)
		c.Mask = 1 << i
		c.Pips[i] = 1
	}
	for a := range c.VPair {
		c.VPair[a] = v
		if c.Mask&^PairMask[a] == 0 {
			c.VPair[a] = v + 0.25
		}
	}
	return c
}

func ref(id, name string) limited.CardRef {
	return limited.CardRef{ID: id, Name: name, Set: "TST", Number: "1", Rarity: limited.Common}
}

// handTable is the golden test's set: two cards per colour, a removal
// spell, a colourless artifact and a land.
func handTable(t testing.TB) *SetTable {
	t.Helper()
	rows := []Card{
		mono("W1", "W", 0.0, TypeCreature), mono("W2", "W", -0.5, TypeCreature),
		mono("U1", "U", 0.5, TypeCreature), mono("U2", "U", -0.2, TypeCreature),
		mono("B1", "B", -0.5, TypeCreature), mono("B2", "B", 0.3, TypeSorcery),
		mono("R1", "R", 2.0, TypeCreature), mono("R2", "R", 1.5, TypeCreature),
		mono("G1", "G", 1.0, TypeCreature), mono("G2", "G", 0.2, TypeCreature),
		mono("Bolt", "R", 1.2, TypeInstant), mono("Golem", "", 0.1, TypeCreature|TypeArtifact),
		mono("Gate", "", -1, TypeLand),
	}
	rows[10].Roles = RoleRemoval
	rows[10].MV = 1
	tab, err := NewSetTable("TST", rows, DefaultModel(), []float64{0.8, 0.6, 0.4, 0.2, 0, -0.2})
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

// handView is a 4-seat, 6-card-pack round-1 history at pick 5: the pick-1
// pack wheels. Seat s took R1 from it; the three downstream seats took R2,
// G1 and U1 (in some order), leaving W1 and B1.
func handView() draft.PickView {
	return draft.PickView{
		Set: "TST", Round: 1, PickNo: 5, Seats: 4,
		Seen: [][]limited.CardRef{
			{ref("a1", "R1"), ref("a2", "R2"), ref("a3", "G1"), ref("a4", "U1"), ref("a5", "W1"), ref("a6", "B1")},
			{ref("b1", "Bolt"), ref("b2", "G2"), ref("b3", "W2"), ref("b4", "U2"), ref("b5", "Gate")},
			{ref("c1", "Golem"), ref("c2", "B2"), ref("c3", "W1"), ref("c4", "U2")},
			{ref("d1", "G2"), ref("d2", "R2"), ref("d3", "B1")},
		},
		Picks: []limited.CardRef{ref("a1", "R1"), ref("b1", "Bolt"), ref("c1", "Golem"), ref("d2", "R2")},
		Pack:  []limited.CardRef{ref("a5", "W1"), ref("a6", "B1")},
	}
}

func TestSetTableJSONRoundTrip(t *testing.T) {
	tab := handTable(t)
	b, err := tab.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeSetTable(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Hash() != tab.Hash() || !reflect.DeepEqual(back.Cards, tab.Cards) || !reflect.DeepEqual(back.Model, tab.Model) {
		t.Fatalf("round trip changed the table: %s vs %s", back.Hash(), tab.Hash())
	}
	if back.Index("R1") < 0 || back.Index("nope") != -1 {
		t.Fatal("Index")
	}
	if _, err := NewSetTable("X", []Card{{Name: "a"}, {Name: "a"}}, DefaultModel(), nil); err == nil {
		t.Fatal("duplicate row accepted")
	}
	m := DefaultModel()
	m.Beta = 0
	if _, err := NewSetTable("X", nil, m, nil); err == nil {
		t.Fatal("beta 0 accepted")
	}
}

// TestGoldenHandBuiltHistory pins hand-checkable features of the 4-seat
// history, then the whole feature set against a golden file.
func TestGoldenHandBuiltHistory(t *testing.T) {
	tab := handTable(t)
	f := Extract(handView(), tab)

	if !f.HaveWheel {
		t.Fatal("the pick-1 pack came back at pick 5 but no wheel was read")
	}
	// R = {R2, G1, U1}: one card each of U, R, G.
	if want := [NColours]float64{0, 1, 0, 1, 1}; f.WheelMass != want {
		t.Fatalf("WheelMass = %v, want %v", f.WheelMass, want)
	}
	// Pool R1, Bolt, Golem, R2: every top-23 sum includes Golem (0.1);
	// red pairs add R1+Bolt+R2 = 4.7, so the best pair is a red one and the
	// first red pair in Pairs order (WR, index 2) wins the tie.
	if f.TopPairs[0] != 2 {
		t.Fatalf("TopPairs = %v, want WR (2) first", f.TopPairs)
	}
	if got := f.PairTop23[2]; math.Abs(got-4.8) > 1e-12 {
		t.Fatalf("WR top-23 sum = %v, want 4.8", got)
	}
	// WR holes: 3 creatures (R1, R2, Golem) → 12 short; 2-drops 3 of 4
	// (Bolt is MV 1); removal 1 of 3.
	if want := [NHoles]float64{12, 1, 4, 2, 0}; f.Holes[0] != want {
		t.Fatalf("Holes[0] = %v, want %v", f.Holes[0], want)
	}
	// Colour commitment: only red cards and Golem are above replacement.
	if f.Colour[3] < 0.99 || f.Mass <= 0 {
		t.Fatalf("Colour = %v mass %v, want all-red", f.Colour, f.Mass)
	}
	// Candidates W1 (0) and B1 (−0.5): W1 ranks first; neither wheels (2 <= n).
	if len(f.Cands) != 2 || f.Cands[0].Rank != 0 || f.Cands[1].Rank != 1 || f.Cands[0].WheelDefined {
		t.Fatalf("Cands = %+v", f.Cands)
	}
	// W1 is on WR and the pool has < 23 on-pair cards: gain = its value.
	if f.Cands[0].Gain[0] != 0 {
		t.Fatalf("W1 gain on WR = %v, want 0 (v=0)", f.Cands[0].Gain[0])
	}
	if f.Cands[1].Gain[0] != 0 {
		t.Fatalf("B1 gain on WR = %v, want 0 (off pair)", f.Cands[1].Gain[0])
	}
	// The three downstream seats took R2, G1 and U1 and all passed W1 and
	// B1: no downstream posterior favours WB (pair 1), and every row is a
	// distribution.
	for o := 0; o < 3; o++ {
		row := f.Posterior[o]
		s := 0.0
		for a := range row {
			s += row[a]
		}
		if math.Abs(s-1) > 1e-9 {
			t.Fatalf("posterior %d sums to %v", o, s)
		}
		if row[1] >= 0.1 {
			t.Fatalf("seat +%d: WB mass %v, want below uniform after passing W1 and B1", o+1, row[1])
		}
	}
	for o := 3; o < MaxSeats-1; o++ {
		if f.Posterior[o] != ([NPairs]float64{}) {
			t.Fatalf("row %d beyond the pod is not zero", o)
		}
	}

	got, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	const golden = "testdata/golden_4seat.json"
	if *update {
		if err := os.WriteFile(golden, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (regenerate with -update)", err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("features drifted from %s (regenerate with -update if intended):\n%s", golden, got)
	}
}

// bruteWheel is the reference: enumerate every permutation of R, score
// each seat's softmax over its pack state from scratch.
func bruteWheel(e *Extractor, nr int) [maxRemoved][NPairs]float64 {
	w := &e.w
	var acc [maxRemoved][NPairs]float64
	perm := make([]int, nr)
	for i := range perm {
		perm[i] = i
	}
	var rec func(int)
	rec = func(d int) {
		if d == nr {
			removed := map[int]bool{}
			var wts [maxRemoved][NPairs]float64
			W := 1.0
			for m := 0; m < nr; m++ {
				c := w.rpos[perm[m]]
				var z float64
				for a := 0; a < NPairs; a++ {
					den := 0.0
					for pos, ci := range w.s0 {
						if removed[pos] {
							continue
						}
						den += e.score(ci, a, w.kap[m])
					}
					wts[m][a] = e.q[w.off[m]-1][a] * e.score(w.s0[c], a, w.kap[m]) / den
					z += wts[m][a]
				}
				for a := range wts[m] {
					wts[m][a] /= z
				}
				W *= z
				removed[c] = true
			}
			for m := 0; m < nr; m++ {
				for a := 0; a < NPairs; a++ {
					acc[m][a] += W * wts[m][a]
				}
			}
			return
		}
		for i := d; i < nr; i++ {
			perm[d], perm[i] = perm[i], perm[d]
			rec(d + 1)
			perm[d], perm[i] = perm[i], perm[d]
		}
	}
	rec(0)
	for m := 0; m < nr; m++ {
		s := 0.0
		for _, x := range acc[m] {
			s += x
		}
		for a := range acc[m] {
			acc[m][a] /= s
		}
	}
	return acc
}

// TestWheelUpdateMatchesBruteForce: the depth-first update equals the
// naive per-permutation enumeration for n = 4 and n = 6.
func TestWheelUpdateMatchesBruteForce(t *testing.T) {
	tab := synthTable(t)
	r := rand.New(rand.NewPCG(7, 7))
	for _, n := range []int{4, 6} {
		e := wheelFixture(tab, r, n, 12)
		want := bruteWheel(e, n-1)
		e.wheelUpdate(n - 1)
		for m := 0; m < n-1; m++ {
			got := e.q[e.w.off[m]-1]
			for a := 0; a < NPairs; a++ {
				if math.Abs(got[a]-want[m][a]) > 1e-9 {
					t.Fatalf("n=%d seat %d pair %d: dfs %v, brute %v", n, m, a, got[a], want[m][a])
				}
			}
		}
	}
}

// TestJointWheelMatchesFactorised: from a product prior, one exact joint
// wheel update has the same per-seat marginals as the factorised update
// (they differ only once correlations carry into later wheels).
func TestJointWheelMatchesFactorised(t *testing.T) {
	tab := synthTable(t)
	e := wheelFixture(tab, rand.New(rand.NewPCG(5, 5)), 4, 11)
	prior := e.q
	e.jn = jointSize
	for tu := 0; tu < e.jn; tu++ {
		p := 1.0
		for o := 0; o < 3; o++ {
			p *= prior[o][digit(tu, o)]
		}
		e.jp[tu] = p
	}
	e.jointWheel(3)
	e.jointMarginals(3)
	joint := e.q
	e.q = prior
	e.wheelUpdate(3)
	for o := 0; o < 3; o++ {
		for a := 0; a < NPairs; a++ {
			if math.Abs(joint[o][a]-e.q[o][a]) > 1e-9 {
				t.Fatalf("seat %d pair %d: joint %v, factorised %v", o, a, joint[o][a], e.q[o][a])
			}
		}
	}
}

// wheelFixture loads a random wheel: an s0 of size cards from the synthetic
// table, the first n−1 removed, random seat priors.
func wheelFixture(tab *SetTable, r *rand.Rand, n, size int) *Extractor {
	e := &Extractor{t: tab}
	for i := 0; i < size; i++ {
		e.w.s0 = append(e.w.s0, int32(r.IntN(len(tab.Cards))))
	}
	for j := 0; j < n-1; j++ {
		e.w.rpos = append(e.w.rpos, j*size/(n-1))
	}
	for m := 0; m < n-1; m++ {
		e.w.off[m] = m + 1
		e.w.kap[m] = tab.Model.Kappa
		s := 0.0
		for a := 0; a < NPairs; a++ {
			e.q[m][a] = 0.2 + r.Float64()
			s += e.q[m][a]
		}
		for a := range e.q[m] {
			e.q[m][a] /= s
		}
	}
	return e
}

// synthTable is a 270-card synthetic set: 50 mono cards per colour and 20
// colourless, values drawn once from a fixed seed.
func synthTable(t testing.TB) *SetTable {
	t.Helper()
	r := rand.New(rand.NewPCG(1, 2))
	var rows []Card
	for c := 0; c < NColours; c++ {
		for i := 0; i < 50; i++ {
			rows = append(rows, mono(fmt.Sprintf("%c%02d", colours[c], i), string(colours[c]), r.NormFloat64(), TypeCreature))
		}
	}
	for i := 0; i < 20; i++ {
		rows = append(rows, mono(fmt.Sprintf("C%02d", i), "", r.NormFloat64()-0.5, TypeArtifact))
	}
	tab, err := NewSetTable("SYN", rows, DefaultModel(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

// podResult is one simulated pod's read at the probe pick.
type podResult struct {
	truth [MaxSeats - 1]int
	top   [MaxSeats - 1]int
	feat  Features
}

// simPod drafts one 4-seat pod: seat 0 takes the highest value; seats 1..3
// are scripted to a fixed pair, taking argmax v + 2·fit(c, pair). It
// returns seat 0's features at round 2 pick 5.
func simPod(tab *SetTable, seed uint64) podResult { return simPodN(tab, seed, 4) }

// simPodN is simPod with an n-seat pod.
func simPodN(tab *SetTable, seed uint64, n int) podResult {
	r := rand.New(rand.NewPCG(seed, 99))
	const rounds, size = 3, 14
	packs := make([][][]limited.CardRef, n)
	for s := range packs {
		packs[s] = make([][]limited.CardRef, rounds)
		for rd := range packs[s] {
			for j := 0; j < size; j++ {
				c := tab.Cards[r.IntN(len(tab.Cards))]
				packs[s][rd] = append(packs[s][rd], ref(fmt.Sprintf("%d-%d-%d", s, rd, j), c.Name))
			}
		}
	}
	var res podResult
	for o := 0; o < n-1; o++ {
		res.truth[o] = r.IntN(NPairs)
	}
	p, err := draft.NewPod(seed, packs)
	if err != nil {
		panic(err)
	}
	var ex Extractor
	for !p.Done() {
		for s := 0; s < n; s++ {
			v, ok := p.Next(s)
			if !ok {
				continue
			}
			if s == 0 && v.Round == 2 && v.PickNo == 5 {
				res.feat = ex.Extract(v, tab)
				for o := 0; o < n-1; o++ {
					res.top[o] = argmax(&res.feat.Posterior[o])
				}
				return res
			}
			best, bestScore := 0, math.Inf(-1)
			for i, c := range v.Pack {
				ci := tab.Index(c.Name)
				sc := tab.Cards[ci].V
				if s > 0 {
					sc += 2 * tab.fit[ci][res.truth[s-1]]
				}
				if sc > bestScore {
					best, bestScore = i, sc
				}
			}
			if err := p.Pick(s, v.PickNo, v.Pack[best].ID); err != nil {
				panic(err)
			}
		}
	}
	panic("pod ended before round 2 pick 5")
}

func argmax(x *[NPairs]float64) int {
	b := 0
	for a := 1; a < NPairs; a++ {
		if x[a] > x[b] {
			b = a
		}
	}
	return b
}

func runPods(tab *SetTable, pods int) (hits, total int, feats []Features) {
	for i := 0; i < pods; i++ {
		res := simPod(tab, uint64(1000+i))
		for o := 0; o < 3; o++ {
			total++
			if res.top[o] == res.truth[o] {
				hits++
			}
		}
		feats = append(feats, res.feat)
	}
	return
}

// TestPosteriorRecoversScriptedPairs: over 200 draft-only pods, seat 0's
// posterior names each scripted opponent's pair top-1 at least 80% of the
// time by round 2 pick 5.
//
// The opponent model is the one calibrated to the scripted seats (DR-4's
// calibrate fits it by maximum likelihood; here it is set by hand): they
// commit from pick 1 (no κ ramp) with κ = 2, and β = 4, m = 3 is the fit.
// DefaultModel's uncalibrated ramp reads 67% on the same pods.
func TestPosteriorRecoversScriptedPairs(t *testing.T) {
	tab := synthTable(t)
	tab.Model.Beta, tab.Model.Kappa, tab.Model.KappaRamp, tab.Model.M = 4, 2, 0, []float64{3}
	start := time.Now()
	hits, total, _ := runPods(tab, 200)
	el := time.Since(start)
	acc := float64(hits) / float64(total)
	t.Logf("posterior top-1: %d/%d = %.1f%% in %v", hits, total, 100*acc, el)
	if acc < 0.80 {
		t.Fatalf("top-1 accuracy %.3f < 0.80", acc)
	}
	if el > 20*time.Second {
		t.Fatalf("200 pods took %v, budget 20s", el)
	}
}

// TestDeterminism: the same pods twice give equal features, bit for bit.
func TestDeterminism(t *testing.T) {
	tab := synthTable(t)
	h1, _, f1 := runPods(tab, 20)
	h2, _, f2 := runPods(tab, 20)
	if h1 != h2 || !reflect.DeepEqual(f1, f2) {
		t.Fatal("two identical runs disagree")
	}
	a, b := Extract(handView(), handTable(t)), Extract(handView(), handTable(t))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("Extract is not deterministic on the hand view")
	}
}

// TestEightSeatPod: an 8-seat pod runs the factorised update; every
// other seat's row is a distribution. The top-1 rate is logged, not gated
// (the spec's acceptance is the 4-seat test above).
func TestEightSeatPod(t *testing.T) {
	tab := synthTable(t)
	tab.Model.Beta, tab.Model.Kappa, tab.Model.KappaRamp, tab.Model.M = 4, 2, 0, []float64{3}
	hits, total := 0, 0
	for i := 0; i < 50; i++ {
		res := simPodN(tab, uint64(5000+i), 8)
		if res.feat.Seats != 8 || !res.feat.HaveWheel {
			t.Fatalf("pod %d: seats %d wheel %v", i, res.feat.Seats, res.feat.HaveWheel)
		}
		for o := 0; o < 7; o++ {
			s := 0.0
			for _, x := range res.feat.Posterior[o] {
				s += x
			}
			if math.Abs(s-1) > 1e-9 {
				t.Fatalf("pod %d row %d sums to %v", i, o, s)
			}
			total++
			if res.top[o] == res.truth[o] {
				hits++
			}
		}
	}
	t.Logf("8-seat top-1 at round 2 pick 5: %d/%d = %.1f%%", hits, total, 100*float64(hits)/float64(total))
}

// BenchmarkWheelUpdate8 is the n = 8 exact wheel update (7 removed cards,
// 13-card pack): spec target ≤ 1 ms, not a gate.
func BenchmarkWheelUpdate8(b *testing.B) {
	tab := synthTable(b)
	e := wheelFixture(tab, rand.New(rand.NewPCG(3, 3)), 8, 13)
	q := e.q
	b.ReportAllocs()
	for b.Loop() {
		e.q = q
		e.wheelUpdate(7)
	}
}

// BenchmarkExtract4 is a whole Extract at round 2 pick 5 of a 4-seat pod.
func BenchmarkExtract4(b *testing.B) {
	tab := synthTable(b)
	var ex Extractor
	v := probeView(tab, 1)
	b.ReportAllocs()
	for b.Loop() {
		ex.Extract(v, tab)
	}
}

// probeView replays simPod's pod to seat 0's round 2 pick 5 view.
func probeView(tab *SetTable, seed uint64) draft.PickView {
	r := rand.New(rand.NewPCG(seed, 99))
	const n, rounds, size = 4, 3, 14
	packs := make([][][]limited.CardRef, n)
	for s := range packs {
		packs[s] = make([][]limited.CardRef, rounds)
		for rd := range packs[s] {
			for j := 0; j < size; j++ {
				c := tab.Cards[r.IntN(len(tab.Cards))]
				packs[s][rd] = append(packs[s][rd], ref(fmt.Sprintf("%d-%d-%d", s, rd, j), c.Name))
			}
		}
	}
	p, _ := draft.NewPod(seed, packs)
	for {
		for s := 0; s < n; s++ {
			v, ok := p.Next(s)
			if !ok {
				continue
			}
			if s == 0 && v.Round == 2 && v.PickNo == 5 {
				return v
			}
			_ = p.Pick(s, v.PickNo, v.Pack[0].ID)
		}
	}
}
