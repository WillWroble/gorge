package draftnet

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draft"
	"github.com/adams-shaun/gorge/limited/draftfeat"
)

// parityCase is one fixture pick, as draftrl/py/gen_fixture.py writes it.
type parityCase struct {
	Ctx   []float32 `json:"ctx"`
	Seats []struct {
		Off  int       `json:"off"`
		Feat []float32 `json:"feat"`
	} `json:"seats"`
	Pool []struct {
		Emb   int32     `json:"emb"`
		Pick  int32     `json:"pick"`
		Facts []float32 `json:"facts"`
	} `json:"pool"`
	Cand []struct {
		Emb   int32     `json:"emb"`
		Facts []float32 `json:"facts"`
		Feat  []float32 `json:"feat"`
	} `json:"cand"`
	Logits []float64 `json:"logits"`
}

func (c *parityCase) inputs() *Inputs {
	in := &Inputs{NSeat: len(c.Seats)}
	copy(in.Ctx[:], c.Ctx)
	for j, s := range c.Seats {
		in.SeatOff[j] = s.Off
		copy(in.Seat[j][:], s.Feat)
	}
	for _, p := range c.Pool {
		t := PoolTok{Card: Card{Emb: p.Emb}, Pick: p.Pick}
		copy(t.Facts[:], p.Facts)
		in.Pool = append(in.Pool, t)
	}
	for _, p := range c.Cand {
		t := CandTok{Card: Card{Emb: p.Emb}}
		copy(t.Facts[:], p.Facts)
		copy(t.Feat[:], p.Feat)
		in.Cand = append(in.Cand, t)
	}
	return in
}

func loadFixture(t testing.TB) (*Net, []parityCase) {
	t.Helper()
	b, err := os.ReadFile("testdata/parity.dpkw")
	if err != nil {
		t.Fatal(err)
	}
	n, err := Load(b)
	if err != nil {
		t.Fatal(err)
	}
	j, err := os.ReadFile("testdata/parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct{ Cases []parityCase }
	if err := json.Unmarshal(j, &fx); err != nil {
		t.Fatal(err)
	}
	return n, fx.Cases
}

// TestParityWithPythonReference: the committed fixture's logits come from
// the torch reference (draftrl/py/model.py); Go must agree within 1e-4.
func TestParityWithPythonReference(t *testing.T) {
	n, cases := loadFixture(t)
	if len(cases) == 0 {
		t.Fatal("fixture has no cases")
	}
	var r Runner
	worst := 0.0
	for i := range cases {
		c := &cases[i]
		got, err := r.Forward(n, c.inputs())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(c.Logits) {
			t.Fatalf("case %d: %d logits, want %d", i, len(got), len(c.Logits))
		}
		for j := range got {
			d := math.Abs(float64(got[j]) - c.Logits[j])
			worst = math.Max(worst, d)
			if d > 1e-4 {
				t.Errorf("case %d cand %d: go %v, python %v", i, j, got[j], c.Logits[j])
			}
		}
	}
	t.Logf("max |go - python| = %.3g over %d cases", worst, len(cases))
}

func TestParamCount(t *testing.T) {
	n, _ := loadFixture(t)
	emb, rest := n.ParamCount()
	t.Logf("params: %d excluding the card embedding (+%d embedding for %d cards)", rest, emb, n.Cards())
	if rest < 60000 || rest > 100000 {
		t.Errorf("policy has %d parameters; spec §5 says about 80k", rest)
	}
}

func fixtureFile(t *testing.T) *File {
	b, err := os.ReadFile("testdata/parity.dpkw")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestEncodeRoundTrip(t *testing.T) {
	b, _ := os.ReadFile("testdata/parity.dpkw")
	f := fixtureFile(t)
	if string(f.Encode()) != string(b) {
		t.Fatal("Decode/Encode does not round-trip the fixture byte for byte")
	}
}

// TestLoadRefusesCritic is spec §2.2: the omniscient critic never reaches
// the policy.
func TestLoadRefusesCritic(t *testing.T) {
	f := fixtureFile(t)
	f.Tensors = append(f.Tensors, Tensor{Name: "critic.v.w", Shape: []int{4}, Data: make([]float32, 4)})
	_, err := Load(f.Encode())
	if err == nil || !strings.Contains(err.Error(), "critic") {
		t.Fatalf("Load accepted a critic tensor (err %v)", err)
	}
}

func TestLoadRefusesUnknownMissingAndMisshapen(t *testing.T) {
	f := fixtureFile(t)
	f.Tensors = append(f.Tensors, Tensor{Name: "extra", Shape: []int{1}, Data: []float32{0}})
	if _, err := Load(f.Encode()); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("unknown tensor: err %v", err)
	}
	f = fixtureFile(t)
	f.Tensors = f.Tensors[1:]
	if _, err := Load(f.Encode()); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("missing tensor: err %v", err)
	}
	f = fixtureFile(t)
	f.Tensors[0].Shape = []int{2, 128}
	if _, err := Load(f.Encode()); err == nil || !strings.Contains(err.Error(), "shape") {
		t.Errorf("misshapen tensor: err %v", err)
	}
	b, _ := os.ReadFile("testdata/parity.dpkw")
	if _, err := Load(b[:len(b)-3]); err == nil {
		t.Error("truncated file loaded")
	}
}

// --- a synthetic set and pod for the Fill path --------------------------

func ref(id, name string) limited.CardRef {
	return limited.CardRef{ID: id, Name: name, Set: "TST", Number: "1", Rarity: limited.Common}
}

func synthTable(t testing.TB) *draftfeat.SetTable {
	t.Helper()
	r := rand.New(rand.NewPCG(3, 4))
	var rows []draftfeat.Card
	for i := 0; i < 60; i++ {
		c := draftfeat.Card{Name: fmt.Sprintf("C%02d", i), Mask: 1 << (i % 5), MV: uint8(1 + i%6),
			Types: draftfeat.TypeCreature, V: r.NormFloat64(), SE: 0.1}
		c.Pips[i%5] = 1
		for a := range c.VPair {
			c.VPair[a] = c.V + 0.3*r.NormFloat64()
		}
		rows = append(rows, c)
	}
	tab, err := draftfeat.NewSetTable("TST", rows, draftfeat.DefaultModel(), []float64{0.5, 0.3, 0.1, 0})
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

// podView replays a 4-seat, 14-card pod to seat 0's (round, pick) view.
func podView(tab *draftfeat.SetTable, round, pick int) draft.PickView {
	r := rand.New(rand.NewPCG(9, 9))
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
	p, _ := draft.NewPod(1, packs)
	for {
		for s := 0; s < n; s++ {
			v, ok := p.Next(s)
			if !ok {
				continue
			}
			if s == 0 && v.Round == round && v.PickNo == pick {
				return v
			}
			_ = p.Pick(s, v.PickNo, v.Pack[0].ID)
		}
	}
}

func TestFillLayout(t *testing.T) {
	tab := synthTable(t)
	v := podView(tab, 2, 5)
	f := draftfeat.Extract(v, tab)
	var in Inputs
	in.Fill(&f, v, tab)
	if in.NSeat != 3 || len(in.Pool) != len(v.Picks) || len(in.Cand) != len(v.Pack) {
		t.Fatalf("tokens: seats %d pool %d cand %d", in.NSeat, len(in.Pool), len(in.Cand))
	}
	// Round 2 passes right (d = -1): seat +1 hands us its pack.
	if in.Seat[0][0] != 1 || in.Seat[1][0] != 0 || in.Seat[2][0] != 0 {
		t.Errorf("upstream flags %v %v %v", in.Seat[0][0], in.Seat[1][0], in.Seat[2][0])
	}
	if in.Ctx[3] != -1 || in.Ctx[0] != float32(2)/3 {
		t.Errorf("ctx head %v", in.Ctx[:4])
	}
	for j, c := range in.Cand {
		if int(c.Emb) != tab.Index(v.Pack[j].Name) || c.Feat[0] != float32(f.Cands[j].V) {
			t.Errorf("cand %d misencoded", j)
		}
	}
}

// TestForwardAllocatesNothing: after warm-up a pick's Fill+Forward is
// allocation free (operator perf rule).
func TestForwardAllocatesNothing(t *testing.T) {
	n, _ := loadFixture(t)
	tab := synthTable(t)
	v := podView(tab, 2, 5)
	f := draftfeat.Extract(v, tab)
	var in Inputs
	var r Runner
	in.Fill(&f, v, tab)
	if _, err := r.Forward(n, &in); err != nil {
		t.Fatal(err)
	}
	if a := testing.AllocsPerRun(20, func() {
		in.Fill(&f, v, tab)
		_, _ = r.Forward(n, &in)
	}); a != 0 {
		t.Fatalf("Fill+Forward allocates %v per pick", a)
	}
}

// BenchmarkForward4 is the forward pass for one pick of a 4-seat pod at
// round 1 pick 1 (14 candidates) and round 2 pick 5 (10 candidates, 18
// pool cards).
func BenchmarkForward4(b *testing.B) {
	n, _ := loadFixture(b)
	tab := synthTable(b)
	for _, rp := range [][2]int{{1, 1}, {2, 5}, {3, 1}} {
		v := podView(tab, rp[0], rp[1])
		f := draftfeat.Extract(v, tab)
		var in Inputs
		in.Fill(&f, v, tab)
		var r Runner
		b.Run(fmt.Sprintf("r%dp%d_tokens%d", rp[0], rp[1], 1+in.NSeat+len(in.Pool)+len(in.Cand)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = r.Forward(n, &in)
			}
		})
	}
}
