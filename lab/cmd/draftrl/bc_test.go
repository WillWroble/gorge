package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/adams-shaun/gorge/internal/testutil"
	"github.com/adams-shaun/gorge/lab/draft/draftlab"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
)

// readPicksBin parses a picks.bin into its header drafter list and, per
// record, (pod, seat, drafterIdx, chosenIdx, logits).
type binPick struct {
	pod, seat, drafter, chosen int
	logits                     []float32
	behaviour                  bool
	temp, logp                 float32
}

func readPicksBin(t *testing.T, path string) ([]string, []binPick) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:4]) != "DRPB" {
		t.Fatalf("magic %q", b[:4])
	}
	if v := binary.LittleEndian.Uint32(b[4:]); v != 1 && v != 2 {
		t.Fatalf("picks.bin version %d", v)
	}
	G, C, nd := int(binary.LittleEndian.Uint32(b[8:])), int(binary.LittleEndian.Uint32(b[12:])), int(binary.LittleEndian.Uint32(b[16:]))
	o := 20
	str := func() string {
		n := int(binary.LittleEndian.Uint16(b[o:]))
		s := string(b[o+2 : o+2+n])
		o += 2 + n
		return s
	}
	str() // table hash
	var names []string
	for i := 0; i < nd; i++ {
		names = append(names, str())
	}
	var out []binPick
	for o < len(b) {
		p := binPick{pod: int(binary.LittleEndian.Uint32(b[o:])), seat: int(b[o+4])}
		size, chosen, has, flags := int(b[o+7]), int(b[o+8]), b[o+10], b[o+11]
		p.drafter, p.chosen = int(b[o+9]), chosen
		o += 12 + 4*G + size*(4+4*C)
		if has == 1 {
			for i := 0; i < size; i++ {
				p.logits = append(p.logits, math.Float32frombits(binary.LittleEndian.Uint32(b[o:])))
				o += 4
			}
		}
		if flags&1 != 0 {
			p.behaviour = true
			p.temp = math.Float32frombits(binary.LittleEndian.Uint32(b[o:]))
			p.logp = math.Float32frombits(binary.LittleEndian.Uint32(b[o+4:]))
			o += 8
		}
		out = append(out, p)
	}
	return names, out
}

// TestBCSmoke is DR-8's smoke: 2 draft-only pods. Every pick carries the
// teacher's scores, an argmax teacher seat picks the teacher's best score,
// every seat's label equals its built deck's top colour pair, and the
// records reproduce from the seed at another worker count.
func TestBCSmoke(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	outA, outB := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	args := func(out, workers string) []string {
		return []string{"-corpus", dir, "-pods", "2", "-teacher", "17l", "-seed", "3000000",
			"-variants", "temp:3,eps:0.15,cfm:4,cfm:12,17l,random", "-workers", workers, "-dump-inputs", "5", "-out", out}
	}
	var log bytes.Buffer
	r, err := bc(args(outA, "2"), &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if r.Picks != 2*4*3*14 || len(r.Seats) != 8 {
		t.Fatalf("picks %d seats %d, want 336/8", r.Picks, len(r.Seats))
	}
	names, picks := readPicksBin(t, filepath.Join(outA, "picks.bin"))
	if len(picks) != r.Picks || names[0] != "teacher" {
		t.Fatalf("picks.bin: %d records, drafters %v", len(picks), names)
	}
	teacherPicks := 0
	for i, p := range picks {
		if len(p.logits) == 0 || p.chosen == 255 {
			t.Fatalf("pick %d: no teacher scores or no choice", i)
		}
		if p.drafter != 0 {
			continue
		}
		teacherPicks++
		best := 0
		for j, x := range p.logits {
			if x > p.logits[best] {
				best = j
			}
		}
		if p.logits[p.chosen] < p.logits[best]-1e-4 {
			t.Fatalf("pick %d: teacher chose %d (score %v), best %d (%v)", i, p.chosen, p.logits[p.chosen], best, p.logits[best])
		}
	}
	if teacherPicks != 2*2*42 {
		t.Fatalf("%d teacher picks, want %d (2 teacher seats per pod)", teacherPicks, 2*2*42)
	}
	lab, err := draftlab.Open(dir, draftlab.Options{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(outA, "seats.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	n := 0
	for sc.Scan() {
		var sr seatRec
		if err := json.Unmarshal(sc.Bytes(), &sr); err != nil {
			t.Fatal(err)
		}
		n++
		// The label is the built deck's top pair: the two colours with the
		// most non-land main cards.
		var cnt [draftfeat.NColours]int
		for _, name := range sr.Main {
			c, ok := lab.Card(name)
			if !ok || c.Faces[0].IsLand() {
				continue
			}
			for k := 0; k < draftfeat.NColours; k++ {
				if c.ColourIdentity()&(1<<uint(k)) != 0 {
					cnt[k]++
				}
			}
		}
		a, b := 0, -1
		for k := 1; k < draftfeat.NColours; k++ {
			if cnt[k] > cnt[a] {
				a = k
			}
		}
		for k := 0; k < draftfeat.NColours; k++ {
			if k != a && (b < 0 || cnt[k] > cnt[b]) {
				b = k
			}
		}
		if a > b {
			a, b = b, a
		}
		if sr.Pair < 0 || draftfeat.Pairs[sr.Pair] != [2]int{a, b} || sr.Colours != cnt || len(sr.Main) != 40 {
			t.Fatalf("pod %d seat %d: label %d %s, deck top pair %d%d counts %v", sr.Pod, sr.Seat, sr.Pair, sr.PairName, a, b, cnt)
		}
	}
	if n != 8 {
		t.Fatalf("seats.jsonl has %d lines", n)
	}
	if _, err := bc(args(outB, "1"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"picks.bin", "seats.jsonl", "inputs.jsonl", "table.json"} {
		a, _ := os.ReadFile(filepath.Join(outA, name))
		b, _ := os.ReadFile(filepath.Join(outB, name))
		if len(a) == 0 || !bytes.Equal(a, b) {
			t.Fatalf("%s: not reproduced from the seed", name)
		}
	}
}
