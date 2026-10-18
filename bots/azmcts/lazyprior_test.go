package azmcts

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/adams-shaun/gorge/bots/gonet"
	"github.com/adams-shaun/gorge/bots/gonet/gonettest"
	"github.com/adams-shaun/gorge/bots/searchprobe"
)

// TestLazyPriorMatchesEager pins walkConfig.lazyPrior: a network search over
// re-dealt worlds, with opponent points, auto-pay and a top-k prior (az-c6's
// shape), returns the same Result -- visits, values, choice and every
// counter -- whether in-walk priors are deferred or computed at every point.
// AZMCTS_TEST_NET=<a .dzgw> runs it on a real network instead of the
// gonettest one.
func TestLazyPriorMatchesEager(t *testing.T) {
	defer searchprobe.SetRedealProbeVerify(searchprobe.SetRedealProbeVerify(false))
	path := os.Getenv("AZMCTS_TEST_NET")
	if path == "" {
		var err error
		if path, err = gonettest.Write(t.TempDir(), "lazy.dzgw"); err != nil {
			t.Fatal(err)
		}
	}
	net, err := gonet.Model(path)
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultOptions()
	opts.Sims, opts.Seed = 100, 7
	opts.AutoPayment, opts.OpponentNodes, opts.OpponentMirror, opts.PriorTopK = true, true, true, 6
	opts.Kinds = FullKinds()
	for _, s := range []uint64{testSeed, testSeed + 1} {
		p := feedPosition(t, testConfig(t, "mono-red-prowess", "mono-blue-tempo", s), 5, 1, nil)
		run := func(eager bool) Result {
			defer func(old bool) { eagerPrior = old }(eagerPrior)
			eagerPrior = eager
			obs := searchprobe.NewCollector(0)
			src, err := NewRedeal(p.input(t, p.e), obs, s, 0)
			if err != nil {
				t.Fatal(err)
			}
			res, err := Search(context.Background(), Root{Engine: p.e, Decision: p.d, Bot: p.bot, Observer: obs}, src, net, opts)
			if err != nil {
				t.Fatal(err)
			}
			return res
		}
		eager, lazy := run(true), run(false)
		if eager.Stats.Completed == 0 {
			t.Fatalf("seed %d: no simulation completed", s)
		}
		if !reflect.DeepEqual(eager, lazy) {
			t.Fatalf("seed %d: lazy prior changed the search\neager %+v\nlazy  %+v", s, eager, lazy)
		}
	}
}
