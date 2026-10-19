package azmcts

import (
	"reflect"
	"testing"

	"github.com/adams-shaun/gorge/bots/searchprobe"
	"github.com/adams-shaun/gorge/engine/rules"
)

// playPooled plays one az game on the redeal source under CachedWorlds with
// the node cache, with the spare pool on or off, and returns the final chain
// head, every searched decision's Diag and the seat.
func playPooled(t *testing.T, cfg rules.Config, pooled bool) (string, []Diag, *Seat) {
	t.Helper()
	prevNo, prevWatch := noSparePool, Watch
	noSparePool = !pooled
	var diags []Diag
	Watch = func(d Diag) { diags = append(diags, d) }
	defer func() { noSparePool, Watch = prevNo, prevWatch }()
	sc := DefaultSeatConfig()
	sc.Search.Sims, sc.Search.CachedWorlds, sc.World = 12, 1, WorldRedeal
	az, err := NewSeat(cfg.Seed^1, nil, sc)
	if err != nil {
		t.Fatal(err)
	}
	head, err := playAZ(t, cfg, az, 400)
	if err != nil {
		t.Fatal(err)
	}
	return head, diags, az
}

// TestSparePoolIsExact pins the spare pool (sparePool, fixedEnvs.Reclaim):
// a seat whose cached-worlds trees resume into storage recycled from the
// previous decision's stored states plays exactly the game a seat
// allocating fresh storage plays -- the same chain head, and at every
// searched decision the same choice, candidates and every Stats counter,
// the walk's cost counters included (the pool changes what a clone's
// arrays are, never what the tree does). The pooled seat must actually
// have pooled: its pool holds the last tree's states afterwards.
func TestSparePoolIsExact(t *testing.T) {
	defer searchprobe.SetRedealProbeVerify(searchprobe.SetRedealProbeVerify(false))
	for _, s := range []uint64{testSeed, testSeed + 3} {
		cfg := testConfig(t, "mono-red-prowess", "mono-blue-tempo", s)
		head0, diags0, _ := playPooled(t, cfg, false)
		head1, diags1, az := playPooled(t, cfg, true)
		if head0 != head1 {
			t.Fatalf("seed %d: the pool changed the game: head %s vs %s", s, head0, head1)
		}
		if len(diags0) != len(diags1) {
			t.Fatalf("seed %d: %d vs %d searched decisions", s, len(diags0), len(diags1))
		}
		searched, resumes := 0, 0
		for i := range diags0 {
			a, b := diags0[i], diags1[i]
			a.MS, b.MS = 0, 0
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("seed %d: decision %d differs:\n%+v\nvs\n%+v", s, i, a, b)
			}
			if a.Searched {
				searched++
				resumes += a.Stats.NodeResumes
			}
		}
		if searched < 3 || resumes == 0 {
			t.Fatalf("seed %d: %d searched decisions, %d resumes: the trees never resumed a stored state", s, searched, resumes)
		}
		if az.pool == nil || len(az.pool.spares) == 0 {
			t.Fatalf("seed %d: the pooled seat pooled nothing", s)
		}
		t.Logf("seed %d: %d searched, %d resumes, %d spares pooled", s, searched, resumes, len(az.pool.spares))
	}
}

// TestSparePoolGetPicksTheBestFit pins get's choice: the smallest pooled
// Spare whose event array fits the clone; the largest when none fits; and
// every pooled Spare is detached from its provenance.
func TestSparePoolGetPicksTheBestFit(t *testing.T) {
	cfg := testConfig(t, "mono-red-prowess", "mono-blue-tempo", testSeed)
	e := rules.New(cfg)
	e.Advance()
	// Clones with event arrays of several capacities: a clone's first
	// append grows its array to len + max(256, len/8), so each clone that
	// appends k events releases an array of a different, known size.
	spare := func(n int) rules.Spare {
		c := e.Clone()
		for i := 0; i < n; i++ {
			c.L.Append(c.L.Events[0])
		}
		return c.Release()
	}
	p := newSparePool(8)
	caps := []int{}
	for _, n := range []int{1, 300, 600} {
		sp := spare(n)
		caps = append(caps, sp.EventCap())
		p.put(sp)
	}
	for i := range p.spares {
		if from, n := p.spares[i].Provenance(); from != nil || n != 0 {
			t.Fatalf("pooled spare %d keeps its provenance (%p, %d)", i, from, n)
		}
	}
	// The base engine's clone needs len(e.L.Events)+256: every array fits,
	// and get takes the smallest.
	if got := p.get(e); got.EventCap() != caps[0] {
		t.Fatalf("best fit: got cap %d, want the smallest fitting %d (of %v)", got.EventCap(), caps[0], caps)
	}
	// An engine whose log outgrows every pooled array: get takes the
	// largest.
	big := e.Clone()
	for i := 0; i < 2000; i++ {
		big.L.Append(big.L.Events[0])
	}
	if got := p.get(big); got.EventCap() != caps[2] {
		t.Fatalf("no fit: got cap %d, want the largest %d (of %v)", got.EventCap(), caps[2], caps)
	}
	if got := p.get(e); got.EventCap() != caps[1] {
		t.Fatalf("last: got cap %d, want %d", got.EventCap(), caps[1])
	}
	if got, nilGot := p.get(e), (*sparePool)(nil).get(e); !got.Empty() || !nilGot.Empty() {
		t.Fatal("an empty or nil pool must hand out the zero Spare")
	}
	(*sparePool)(nil).put(spare(1)) // a nil pool drops what it is given
}
