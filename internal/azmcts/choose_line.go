package azmcts

import (
	"math"
	"math/rand/v2"
)

// chooseMove is the root move choice: choose (argmax, or visit-proportional
// when Sample) unless the opt-in randomised-line knobs are set. Temp samples
// by visits^(1/Temp) on every decision; Second then swaps the pick for the
// second most-visited move with probability Second. Both draw from streams
// derived from Seed and ChoiceSalt only, so a line is a pure function of the
// game seed and the arm's salt. With Temp and Second at 0 it is choose.
func chooseMove(visits []int, opts Options, rng *rand.Rand) int {
	pick := choose(visits, opts.Sample && opts.Temp <= 0, rng)
	if opts.Temp > 0 && rng != nil {
		pick = sampleTemp(visits, opts.Temp, rng, pick)
	}
	if opts.Second > 0 {
		r2 := rand.New(rand.NewPCG(opts.Seed^opts.ChoiceSalt^0x5ec0d5ec0d5ec0d5, opts.Seed+0x9e3779b97f4a7c15))
		if r2.Float64() < opts.Second {
			if s := secondBest(visits); s >= 0 {
				pick = s
			}
		}
	}
	return pick
}

// sampleTemp draws an index with probability visits[i]^(1/temp); fallback
// when nothing has a visit.
func sampleTemp(visits []int, temp float64, rng *rand.Rand, fallback int) int {
	w := make([]float64, len(visits))
	total := 0.0
	for i, v := range visits {
		if v > 0 {
			w[i] = math.Pow(float64(v), 1/temp)
			total += w[i]
		}
	}
	if total <= 0 {
		return fallback
	}
	r := rng.Float64() * total
	for i, x := range w {
		if r < x {
			return i
		}
		r -= x
	}
	return fallback
}

// secondBest is the index of the second most-visited move (ties on the lowest
// index, the best being the lowest-index maximum), or -1 when fewer than two
// moves have visits.
func secondBest(visits []int) int {
	best := 0
	for i := 1; i < len(visits); i++ {
		if visits[i] > visits[best] {
			best = i
		}
	}
	sec := -1
	for i, v := range visits {
		if i != best && v > 0 && (sec < 0 || v > visits[sec]) {
			sec = i
		}
	}
	return sec
}
