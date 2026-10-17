package searchprobe

import (
	"testing"

	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestLoadBeliefDirTrainSplitOnly: a pool.tsv naming two train decks and one
// eval deck yields a belief over the two train decks only; without a pool
// every deck file in the directory is read.
func TestLoadBeliefDirTrainSplitOnly(t *testing.T) {
	reg := testutil.CorpusRegistry(t)
	const dir = "testdata/beliefdir"
	b, err := LoadBeliefDir(reg, dir, dir+"/pool.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if b.Decks() != 2 {
		t.Fatalf("train split: got %d decks, want 2", b.Decks())
	}
	all, err := LoadBeliefDir(reg, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if all.Decks() != 3 {
		t.Fatalf("whole dir: got %d decks, want 3", all.Decks())
	}
	if _, err := LoadBeliefDir(reg, dir, dir+"/missing.tsv"); err == nil {
		t.Fatal("missing pool file: want an error")
	}
}
