package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/adams-shaun/gorge/decision"
	"github.com/adams-shaun/gorge/internal/policynet"
	"github.com/adams-shaun/gorge/internal/testutil"
)

// exploreArgs is the label fixture with cast labels (priority decisions, the
// only kind ExploreSeat forces) instead of attackers.
func exploreArgs(t *testing.T, labels string, extra ...string) []string {
	t.Helper()
	a := labelTestArgs(t, labels, 1)
	for i := range a {
		if a[i] == "-kinds" {
			a[i+1] = "cast"
		}
	}
	return append(a, extra...)
}

func TestExploreFlagOnLabels(t *testing.T) {
	testutil.CorpusRegistry(t)
	dir := t.TempDir()

	off := filepath.Join(dir, "off.jsonl")
	if err := run(exploreArgs(t, off), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	zero := filepath.Join(dir, "zero.jsonl")
	if err := run(exploreArgs(t, zero, "-explore", "0"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	offB, _ := os.ReadFile(off)
	zeroB, _ := os.ReadFile(zero)
	if len(offB) == 0 || !bytes.Equal(offB, zeroB) {
		t.Fatalf("-explore 0 differs from the flag absent (%d vs %d bytes)", len(offB), len(zeroB))
	}
	if bytes.Contains(offB, []byte(`"explore"`)) {
		t.Fatal("explore field present with exploration off")
	}
	if len(readLabelRecords(t, off)) == 0 {
		t.Fatal("precondition: no cast labels in the fixture")
	}

	on := filepath.Join(dir, "on.jsonl")
	if err := run(exploreArgs(t, on, "-explore", "1"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	recs := readLabelRecords(t, on)
	flagged := 0
	for _, r := range recs {
		if r.Explore {
			flagged++
			if r.Kind != decision.KPriority {
				t.Fatalf("explore flag on a %s decision; only priority is forced", r.Kind)
			}
		}
	}
	if flagged == 0 {
		t.Fatalf("-explore 1 flagged none of %d cast labels", len(recs))
	}

	// the flag round-trips into the loader: default drops, KeepExplore keeps.
	dropped, st, err := policynet.Load(on)
	if err != nil {
		t.Fatal(err)
	}
	kept, _, err := policynet.LoadWith(on, policynet.LoadOptions{KeepExplore: true})
	if err != nil {
		t.Fatal(err)
	}
	if st.Explored != flagged || len(kept) != len(dropped)+flagged {
		t.Fatalf("loader: dropped %d (Explored=%d), kept %d, flagged %d", len(dropped), st.Explored, len(kept), flagged)
	}
}
