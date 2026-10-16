package main

import (
	"bytes"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adams-shaun/gorge/lab/draft/draftlab"
	"github.com/adams-shaun/gorge/lab/draft/limited/pickbot"

	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftnet"
)

// writeFixtureNet writes the draftnet parity weights re-sized to the
// table in dir/table.json (a zero card embedding, one row per card), with
// meta naming hash.
func writeFixtureNet(t *testing.T, dir string, rows int, hash string) string {
	t.Helper()
	b, err := os.ReadFile("../../draft/limited/draftnet/testdata/parity.dpkw")
	if err != nil {
		t.Fatal(err)
	}
	f, err := draftnet.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	for i := range f.Tensors {
		if f.Tensors[i].Name == "card.emb" {
			f.Tensors[i].Shape = []int{rows, draftnet.CardEmb}
			f.Tensors[i].Data = make([]float32, rows*draftnet.CardEmb)
		}
	}
	f.Meta = []byte(`{"table_hash":"` + hash + `"}`)
	p := filepath.Join(dir, "w.dpkw")
	if err := os.WriteFile(p, f.Encode(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestGateNetDrafter: gate's -drafters takes the registered net kind, with
// the table beside the weights, and refuses weights whose meta names
// another table.
func TestGateNetDrafter(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	data := filepath.Join(t.TempDir(), "bc")
	if _, err := bc([]string{"-corpus", dir, "-pods", "1", "-teacher", "17l", "-out", data}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	tb, err := os.ReadFile(filepath.Join(data, "table.json"))
	if err != nil {
		t.Fatal(err)
	}
	tab, err := decodeTableForTest(tb)
	if err != nil {
		t.Fatal(err)
	}
	w := writeFixtureNet(t, data, len(tab.Cards), tab.Hash())
	var out bytes.Buffer
	r, err := gate([]string{"-corpus", dir, "-events", "1", "-games", "1", "-max-human", "1", "-pilot", "bot", "-baseline", "none",
		"-drafters", "net:" + w, "-builders", "17l"}, &out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if r.BuiltPooled.Games != 4 || !strings.Contains(out.String(), "net:w.dpkw") {
		t.Fatalf("games %d\n%s", r.BuiltPooled.Games, out.String())
	}
	if _, err := gate([]string{"-corpus", dir, "-events", "1", "-drafters", "net:" + w}, &bytes.Buffer{}); err == nil {
		t.Fatal("a net drafter with no -builders was accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad")
	os.MkdirAll(bad, 0o755)
	os.WriteFile(filepath.Join(bad, "table.json"), tb, 0o644)
	wb := writeFixtureNet(t, bad, len(tab.Cards), "0000")
	if _, err := gate([]string{"-corpus", dir, "-events", "1", "-games", "1", "-max-human", "1", "-baseline", "none",
		"-drafters", "net:" + wb, "-builders", "17l"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "trained against") {
		t.Fatalf("weights for another table: err = %v", err)
	}
}

func decodeTableForTest(b []byte) (*draftfeat.SetTable, error) { return draftfeat.DecodeSetTable(b) }

func TestSplitNetTemp(t *testing.T) {
	for _, c := range []struct {
		in, path string
		temp     float64
		bad      bool
	}{
		{"w.dpkw", "w.dpkw", 0, false},
		{"w.dpkw~1", "w.dpkw", 1, false},
		{"d/w.dpkw@t.json~0.5", "d/w.dpkw@t.json", 0.5, false},
		{"~user/w.dpkw", "~user/w.dpkw", 0, false},
		{"a~b/w.dpkw", "a~b/w.dpkw", 0, false},
		{"w.dpkw~-1", "", 0, true},
		{"~1", "", 0, true},
	} {
		p, temp, err := splitNetTemp(c.in)
		if (err != nil) != c.bad || (!c.bad && (p != c.path || temp != c.temp)) {
			t.Errorf("%q: %q %v %v", c.in, p, temp, err)
		}
	}
}

// rngProbe picks the first card and records its seat rng's next draw as
// the first logit, exposing each seat's pick stream.
type rngProbe struct{ last []float32 }

func (f *rngProbe) Pick(v pickbot.PickView, r *rand.Rand) string {
	f.last = make([]float32, len(v.Pack))
	f.last[0] = float32(r.Float64())
	return v.Pack[0].ID
}
func (f *rngProbe) LastLogits() []float32 { return f.last }

// TestSeatPickStreamsDiffer: each seat of a pod draws from its own rng
// stream (draftlab: rand.NewPCG(podSeed, salt|seat)), the stream a
// sampling net seat uses.
func TestSeatPickStreamsDiffer(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	RegisterDrafterKind("rngprobe", func(_ *draftlab.Lab, arg, _ string) (pickbot.Drafter, draftlab.AgentRef, error) {
		return &rngProbe{}, draftlab.AgentRef{Name: "rngprobe"}, nil
	})
	r, err := rollout([]string{"-corpus", dir, "-pods", "1", "-k", "1", "-pilot", "bot", "-workers", "2",
		"-league", "rngprobe:x", "-out", t.TempDir()}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	first := map[int]float32{}
	for _, p := range r.Picks {
		if p.Pick == 0 {
			first[p.Seat] = p.Logits[0]
		}
	}
	seen := map[float32]int{}
	for s, x := range first {
		if o, ok := seen[x]; ok {
			t.Fatalf("seats %d and %d share a pick stream", o, s)
		}
		seen[x] = s
	}
	if len(first) != 4 {
		t.Fatalf("%d seats", len(first))
	}
}

// TestRolloutStochasticNetReproduces: a rollout with S1 net seats at T=1
// records the behaviour log-prob in picks.bin and reproduces byte for byte
// at another worker count. It needs the S1 weights on this box.
func TestRolloutStochasticNetReproduces(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	w := "/mnt/sata/gorge-training/draftrl/s1/final/s1.dpkw"
	if _, err := os.Stat(w); err != nil {
		t.Skip("S1 weights not on this box")
	}
	run := func(workers string) string {
		out := filepath.Join(t.TempDir(), "o"+workers)
		if _, err := rollout([]string{"-corpus", dir, "-pods", "1", "-k", "1", "-pilot", "bot", "-workers", workers,
			"-league", "net:" + w + "~1,net:" + w + ",17l,net:" + w + "~1", "-league-fixed", "-out", out}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a, b := run("1"), run("2")
	for _, f := range []string{"picks.bin", "picks.jsonl", "seats.jsonl"} {
		x, _ := os.ReadFile(filepath.Join(a, f))
		y, _ := os.ReadFile(filepath.Join(b, f))
		if len(x) == 0 || !bytes.Equal(x, y) {
			t.Fatalf("%s differs across worker counts", f)
		}
	}
	_, picks := readPicksBin(t, filepath.Join(a, "picks.bin"))
	nb := 0
	for _, p := range picks {
		sampled := p.seat == 0 || p.seat == 3
		if p.behaviour != sampled {
			t.Fatalf("seat %d: behaviour block %v", p.seat, p.behaviour)
		}
		if !sampled {
			continue
		}
		nb++
		if p.temp != 1 || len(p.logits) == 0 {
			t.Fatalf("seat %d: temp %v logits %d", p.seat, p.temp, len(p.logits))
		}
		want := float32(pickbot.SoftmaxLogProb(p.logits, 1, p.chosen))
		if p.logp > 0 || math.Abs(float64(p.logp-want)) > 1e-6 {
			t.Fatalf("seat %d: logp %v, softmax says %v", p.seat, p.logp, want)
		}
	}
	if nb != 2*3*14 {
		t.Fatalf("%d behaviour records", nb)
	}
}
