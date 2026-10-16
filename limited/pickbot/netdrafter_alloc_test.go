package pickbot

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draft"
)

// captureView is a recorder-less drafter that keeps the view of one pick.
type captureView struct {
	at   int
	n    int
	view PickView
}

func (c *captureView) Pick(v PickView, r *rand.Rand) string {
	if c.n == c.at {
		c.view = v
	}
	c.n++
	return v.Pack[0].ID
}

// TestNetDrafterPickAllocatesNothing: with ExtractInto and the pooled
// scratch, a warm NetDrafter pick allocates nothing (DR-8 known fix).
func TestNetDrafterPickAllocatesNothing(t *testing.T) {
	net, tab := netFixture(t)
	r := rand.New(rand.NewPCG(9, 7))
	packs := make([][][]limited.CardRef, 4)
	for s := range packs {
		packs[s] = make([][]limited.CardRef, 3)
		for rd := range packs[s] {
			for j := 0; j < 14; j++ {
				c := tab.Cards[r.IntN(len(tab.Cards))]
				packs[s][rd] = append(packs[s][rd], limited.CardRef{ID: fmt.Sprintf("%d-%d-%d", s, rd, j), Name: c.Name, Set: "TST", Rarity: limited.Common})
			}
		}
	}
	p, err := draft.NewPod(9, packs)
	if err != nil {
		t.Fatal(err)
	}
	cv := &captureView{at: 17} // seat 0, round 2 pick 4: a part-built pool
	for progress := true; progress; {
		progress = false
		for s := 0; s < 4; s++ {
			v, ok := p.Next(s)
			if !ok {
				continue
			}
			id := v.Pack[0].ID
			if s == 0 {
				id = cv.Pick(v, nil)
			}
			if err := p.Pick(s, v.PickNo, id); err != nil {
				t.Fatal(err)
			}
			progress = true
		}
	}
	if cv.view.Pack == nil {
		t.Fatal("no view captured")
	}
	for _, temp := range []float64{0, 1} {
		d := &NetDrafter{Net: net, Table: tab, Temperature: temp}
		rng := rand.New(rand.NewPCG(1, 1))
		d.Pick(cv.view, rng)
		if a := testing.AllocsPerRun(50, func() { d.Pick(cv.view, rng) }); a != 0 {
			t.Fatalf("T=%v: NetDrafter.Pick allocates %.1f times per pick, want 0", temp, a)
		}
	}
}

// TestRatingColourScoresMatchPick: Pick is exactly BestScore over Scores.
func TestRatingColourScoresMatchPick(t *testing.T) {
	d := &RatingColour{}
	pack := []limited.CardRef{{ID: "a", Name: "Llanowar Elves", Set: "FDN"}, {ID: "b", Name: "Shivan Dragon", Set: "FDN"}, {ID: "c", Name: "Giant Growth", Set: "FDN"}}
	v := PickView{Set: "FDN", Round: 2, PickNo: 3, Pack: pack}
	sc := d.Scores(v, nil)
	if len(sc) != len(pack) {
		t.Fatalf("%d scores for %d cards", len(sc), len(pack))
	}
	r1, r2 := rand.New(rand.NewPCG(3, 3)), rand.New(rand.NewPCG(3, 3))
	if got, want := d.Pick(v, r1), pack[BestScore(sc, r2)].ID; got != want {
		t.Fatalf("Pick %s, BestScore %s", got, want)
	}
	if BestScore(nil, nil) != -1 {
		t.Fatal("BestScore of nothing")
	}
}
