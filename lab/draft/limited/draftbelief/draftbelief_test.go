package draftbelief

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/adams-shaun/gorge/lab/draft/limited"
	"github.com/adams-shaun/gorge/lab/draft/limited/draft"
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat"
)

// synthPod drafts a synthetic 4×3 pod of 14-card packs with simple
// colour-biased drafters and returns seat 0's observation log, the pod
// and the table.
func synthPod(t *testing.T, seed uint64) (Obs, *draft.Pod, *draftfeat.SetTable) {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 1))
	var rows []draftfeat.Card
	for i := 0; i < 80; i++ {
		rows = append(rows, draftfeat.Card{
			Name: fmt.Sprintf("c%02d", i), Mask: 1 << (i % 5), V: r.NormFloat64(), Rarity: limited.Common,
		})
	}
	tab, err := draftfeat.NewSetTable("TST", rows, draftfeat.DefaultModel(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var packs [draft.Seats][draft.Rounds][]limited.CardRef
	id := 0
	for s := 0; s < draft.Seats; s++ {
		for rd := 0; rd < draft.Rounds; rd++ {
			for k := 0; k < 14; k++ {
				c := rows[r.IntN(len(rows))]
				packs[s][rd] = append(packs[s][rd], limited.CardRef{ID: fmt.Sprintf("x%d", id), Name: c.Name, Set: "TST", Rarity: limited.Common})
				id++
			}
		}
	}
	pod := draft.New(seed, packs)
	obs := Obs{Seats: draft.Seats}
	for !pod.Done() {
		for s := 0; s < draft.Seats; s++ {
			v, ok := pod.Next(s)
			if !ok {
				continue
			}
			best, bid := -1e9, ""
			for _, c := range v.Pack {
				ci := tab.Index(c.Name)
				sc := tab.Cards[ci].V
				if tab.Cards[ci].Mask == 1<<uint(s) {
					sc += 1.5
				}
				if sc > best {
					best, bid = sc, c.ID
				}
			}
			if s == 0 {
				obs.Add(v, bid)
			}
			if err := pod.Pick(s, v.PickNo, bid); err != nil {
				t.Fatal(err)
			}
		}
	}
	return obs, pod, tab
}

func owners(pod *draft.Pod) map[string]int {
	m := map[string]int{}
	for s := 0; s < draft.Seats; s++ {
		for _, c := range pod.Pool(s) {
			m[c.ID] = s
		}
	}
	return m
}

// TestCardFlowBookkeeping: every removed set is exactly one card per
// downstream seat it names, and every card of the pod is the observer's
// own, in a removed set, or one of the counted unseen picks.
func TestCardFlowBookkeeping(t *testing.T) {
	for seed := uint64(1); seed <= 5; seed++ {
		obs, pod, tab := synthPod(t, seed)
		f, err := Analyze(obs, tab, Options{})
		if err != nil {
			t.Fatal(err)
		}
		own := owners(pod)
		inSets := 0
		for _, rs := range f.sets {
			got := map[int]int{}
			for _, c := range rs.refs {
				got[own[c.ID]]++
			}
			for _, o := range rs.offs {
				if got[o] != 1 {
					t.Fatalf("seed %d: removed set offs %v, true takers %v", seed, rs.offs, got)
				}
			}
			inSets += len(rs.refs)
		}
		unseen := 0
		for o := 1; o < 4; o++ {
			_, u := f.Taken(o)
			unseen += u
			if want := []int{0, 5, 6, 7}[o]; u != want {
				t.Fatalf("seed %d: offset %d has %d unseen picks, want %d", seed, o, u, want)
			}
		}
		if total := 42 + inSets + unseen; total != 4*3*14 {
			t.Fatalf("seed %d: %d own + %d observed + %d unseen = %d, want 168", seed, 42, inSets, unseen, total)
		}
	}
}

// TestSamplersGiveFullPools: every sampler gives each other seat exactly
// 42 cards, and the same seed gives the same pools.
func TestSamplersGiveFullPools(t *testing.T) {
	obs, _, tab := synthPod(t, 7)
	f, err := Analyze(obs, tab, Options{})
	if err != nil {
		t.Fatal(err)
	}
	f.MeanField(DefaultPool)
	samplers := map[string]func(o int, r *rand.Rand) [][]limited.CardRef{
		"indep": func(o int, r *rand.Rand) [][]limited.CardRef { return f.Sample(o, 8, nil, r) },
		"gibbs": func(o int, r *rand.Rand) [][]limited.CardRef { return f.SampleGibbs(o, 8, nil, DefaultGibbs, r) },
		"exact": func(o int, r *rand.Rand) [][]limited.CardRef { return f.SampleExact(o, 8, nil, r) },
		"mf":    func(o int, r *rand.Rand) [][]limited.CardRef { return f.SampleMF(o, 8, nil, r) },
		"smc": func(o int, r *rand.Rand) [][]limited.CardRef {
			return f.SampleSMC(o, 8, nil, SMCOptions{Particles: 64, Beta: 8, Kappa: 1.5}, r)
		},
	}
	for name, sample := range samplers {
		for o := 1; o < 4; o++ {
			a := sample(o, rand.New(rand.NewPCG(3, uint64(o))))
			b := sample(o, rand.New(rand.NewPCG(3, uint64(o))))
			for i := range a {
				if len(a[i]) != 42 {
					t.Fatalf("%s offset %d: pool of %d cards, want 42", name, o, len(a[i]))
				}
				if fmt.Sprint(a[i]) != fmt.Sprint(b[i]) {
					t.Fatalf("%s offset %d: not deterministic", name, o)
				}
			}
		}
	}
}

// TestKnownCardsLandInThePool: a card the observer saw seat o play is in
// every pool sampled for o.
func TestKnownCardsLandInThePool(t *testing.T) {
	obs, pod, tab := synthPod(t, 11)
	f, err := Analyze(obs, tab, Options{})
	if err != nil {
		t.Fatal(err)
	}
	pool1 := pod.Pool(1)
	known := []string{pool1[5].Name, pool1[20].Name, pool1[35].Name}
	var c Constraints
	c.Known[0] = known
	for _, pool := range f.SampleSMC(1, 16, &c, SMCOptions{Particles: 64, Beta: 8, Kappa: 1.5}, rand.New(rand.NewPCG(1, 2))) {
		have := map[string]int{}
		for _, x := range pool {
			have[x.Name]++
		}
		for _, k := range known {
			if have[k] == 0 {
				t.Fatalf("known card %s missing from a sampled pool", k)
			}
			have[k]--
		}
	}
}

// TestSMCRecoversTheAssignment: on a pod whose drafters follow the pool
// model, the particle posterior puts most mass on the true takers.
func TestSMCRecoversTheAssignment(t *testing.T) {
	obs, pod, tab := synthPod(t, 14)
	f, err := Analyze(obs, tab, Options{})
	if err != nil {
		t.Fatal(err)
	}
	own := owners(pod)
	hit, n := 0.0, 0
	for o := 1; o < 4; o++ {
		pools := f.SampleSMC(o, 64, nil, SMCOptions{Particles: 256, Runs: 8, Beta: 8, Kappa: 3}, rand.New(rand.NewPCG(5, uint64(o))))
		for _, pool := range pools {
			for _, c := range pool {
				if c.ID[0] == 'u' {
					continue
				}
				n++
				if own[c.ID] == o {
					hit++
				}
			}
		}
	}
	if acc := hit / float64(n); acc < 0.6 {
		t.Fatalf("sampled observed cards belong to the right seat %.2f of the time, want >= 0.6", acc)
	}
}
