package main

import "testing"

func TestParsePL3Keys(t *testing.T) {
	base, err := parsePolicy("az:sims=5:explore:nonoise")
	if err != nil {
		t.Fatal(err)
	}
	c0 := base.azConfig()
	if c0.Search.Temp != 0 || c0.Search.Second != 0 || c0.Search.ChoiceSalt != 0 || c0.ExploreTurns != 4 {
		t.Fatalf("defaults moved: %+v turns %d", c0.Search, c0.ExploreTurns)
	}
	p, err := parsePolicy("az:sims=5:explore:nonoise:exploreturns=8:temp=0.5:second=0.15:aseed=9")
	if err != nil {
		t.Fatal(err)
	}
	c := p.azConfig()
	if c.ExploreTurns != 8 || c.Search.Temp != 0.5 || c.Search.Second != 0.15 || c.Search.ChoiceSalt != 9 {
		t.Fatalf("keys not wired: turns %d %+v", c.ExploreTurns, c.Search)
	}
	for _, bad := range []string{"az:sims=5:temp=0", "az:sims=5:temp=-1", "az:sims=5:second=0", "az:sims=5:second=1.5",
		"az:sims=5:exploreturns=-1", "bot:temp=1", "bot:second=0.1", "bot:aseed=3", "az:sims=5:temp=x"} {
		if _, err := parsePolicy(bad); err == nil {
			t.Errorf("%s parsed", bad)
		}
	}
}
