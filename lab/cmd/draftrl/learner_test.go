package main

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestSeatPodLearners checks the S2 pod draw (spec §3.3): exactly
// `learners` seats take the learner, the league draw never returns it,
// a zero-weight entry is never drawn, the weights shape the draw, and with
// no learner and no weights it is seatLeague's uniform draw unchanged.
func TestSeatPodLearners(t *testing.T) {
	const n = 4 // entries 0-2 league, 3 the learner
	weights := []float64{3, 1, 0, 0}
	var counts [n]int
	for seed := uint64(0); seed < 4000; seed++ {
		s := seatPod(n, seed, false, weights, 3, 2)
		got := 0
		for _, e := range s {
			if e == 3 {
				got++
			}
			counts[e]++
		}
		if got != 2 {
			t.Fatalf("seed %d: %v has %d learner seats, want 2", seed, s, got)
		}
	}
	if counts[2] != 0 {
		t.Fatalf("zero-weight entry drawn %d times", counts[2])
	}
	if r := float64(counts[0]) / float64(counts[1]); r < 2.6 || r > 3.4 {
		t.Fatalf("weight 3:1 drew %d:%d (ratio %.2f)", counts[0], counts[1], r)
	}
	for seed := uint64(0); seed < 200; seed++ {
		r := rand.New(rand.NewPCG(seed, leagueSalt))
		var want [4]int
		for s := range want {
			want[s] = r.IntN(5)
		}
		if got := seatPod(5, seed, false, nil, -1, 0); got != want {
			t.Fatalf("seed %d: no-learner draw %v, want the uniform %v", seed, got, want)
		}
	}
	if _, err := parseWeights("1,2", 3); err == nil {
		t.Fatal("2 weights for 3 entries parsed")
	}
	if _, err := parseWeights("0,0", 2); err == nil {
		t.Fatal("zero-sum weights parsed")
	}
}

// TestRolloutLearnerSeats runs a 1-pod rollout with a learner: two seats
// carry the learner spec, the manifest names its drafter index, and
// -picks-jsonl=false leaves picks.jsonl out.
func TestRolloutLearnerSeats(t *testing.T) {
	dir := testutil.CorpusDirOrSkip(t)
	RegisterDrafterKind("fake", fakeKind)
	out := t.TempDir()
	r, err := rollout([]string{"-corpus", dir, "-pods", "1", "-k", "1", "-pilot", "bot", "-seed", "1000000",
		"-league", "17l,fake:v0", "-league-weights", "1,0", "-learner", "fake:v1", "-learners", "2",
		"-picks-jsonl=false", "-out", out}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	learners := 0
	for _, sr := range r.Seats {
		switch sr.Drafter {
		case "fake:v1":
			learners++
		case "17l":
		default:
			t.Fatalf("seat %d drafter %q (zero-weight or unknown entry)", sr.Seat, sr.Drafter)
		}
	}
	if learners != 2 {
		t.Fatalf("%d learner seats, want 2", learners)
	}
	if _, err := os.Stat(filepath.Join(out, "picks.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("picks.jsonl written with -picks-jsonl=false (%v)", err)
	}
	b, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m rolloutManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Learner != "fake:v1" || m.LearnerIdx != 2 || m.Learners != 2 || len(m.League) != 3 || m.League[2] != "fake:v1" {
		t.Fatalf("manifest learner %q idx %d learners %d league %v", m.Learner, m.LearnerIdx, m.Learners, m.League)
	}
	if _, err := parseRollout([]string{"-league", "17l", "-learner", "17l", "-out", out}); err == nil {
		t.Fatal("a learner that is also a league entry parsed")
	}
}
