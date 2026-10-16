package azmcts

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func lineRun(visits []int, o Options, n int) []int {
	out := make([]int, n)
	for i := range out {
		o.Seed = uint64(i)*7919 + 1
		out[i] = chooseMove(visits, o, rand.New(rand.NewPCG(o.Seed^o.ChoiceSalt, o.Seed^0x9e3779b97f4a7c15)))
	}
	return out
}

func TestChooseMoveDefaultIsChoose(t *testing.T) {
	v := []int{3, 9, 9, 1}
	for _, sample := range []bool{false, true} {
		o := Options{Sample: sample}
		for i := 0; i < 50; i++ {
			o.Seed = uint64(i) + 1
			a := rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15))
			b := rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15))
			if got, want := chooseMove(v, o, a), choose(v, sample, b); got != want {
				t.Fatalf("sample=%v seed %d: %d != %d", sample, o.Seed, got, want)
			}
		}
	}
}

func TestChooseMoveTempAndSecondArePureInSeed(t *testing.T) {
	v := []int{40, 30, 20, 10}
	for _, o := range []Options{{Temp: 1}, {Temp: 0.5}, {Second: 0.3}} {
		a, b := lineRun(v, o, 200), lineRun(v, o, 200)
		if !slices.Equal(a, b) {
			t.Fatalf("%+v: not a pure function of the seed", o)
		}
		o.ChoiceSalt = 12345
		if slices.Equal(a, lineRun(v, o, 200)) {
			t.Fatalf("%+v: salt changed nothing", o)
		}
		if distinct(a) < 2 {
			t.Fatalf("%+v: no randomness (picks %v)", o, a[:10])
		}
	}
}

func distinct(x []int) int {
	m := map[int]bool{}
	for _, v := range x {
		m[v] = true
	}
	return len(m)
}

func TestChooseMoveSecond(t *testing.T) {
	v := []int{5, 40, 30, 0}
	got := lineRun(v, Options{Second: 1}, 20)
	for _, g := range got {
		if g != 2 {
			t.Fatalf("Second=1 must always play the second best (2): %v", got)
		}
	}
	// only one visited move: nothing to swap to.
	for _, g := range lineRun([]int{0, 7, 0}, Options{Second: 1}, 20) {
		if g != 1 {
			t.Fatal("swapped to an unvisited move")
		}
	}
	// frequency near p
	n := 0
	for _, g := range lineRun([]int{50, 30}, Options{Second: 0.15}, 4000) {
		if g == 1 {
			n++
		}
	}
	if n < 400 || n > 800 {
		t.Fatalf("Second=0.15 swapped %d/4000", n)
	}
}

func TestChooseMoveTempSharpens(t *testing.T) {
	v := []int{60, 40}
	cnt := func(temp float64) int {
		n := 0
		for _, g := range lineRun(v, Options{Temp: temp}, 4000) {
			if g == 0 {
				n++
			}
		}
		return n
	}
	hi, lo := cnt(1), cnt(0.5) // 0.6 vs 0.69
	if !(lo > hi+150) {
		t.Fatalf("T=0.5 best share %d not above T=1 %d", lo, hi)
	}
}
