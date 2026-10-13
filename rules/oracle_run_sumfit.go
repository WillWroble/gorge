package rules

import "github.com/adams-shaun/gorge/decision"

// oracleSumFitChoices is the runner fallback's answer for a KChoose that
// carries a cumulative constraint (Decision.MinSum, or a present MaxSum
// budget) when the plain "first Min options" default breaks it: the first
// subset in (size, then option order) that satisfies Min/Max and every sum
// bound. A group exile election (Fabrication Foundry's "total mana value X",
// Baron Helmut Zemo's "fifteen or more black mana symbols") is the user; the
// option lists are small, and a decision whose default already holds is
// answered by the default, so no existing run changes. ok is false when no
// subset of at most oracleSumFitMax options satisfies the decision.
func oracleSumFitChoices(d *decision.Decision, def []int) ([]int, bool) {
	if d.Kind != decision.KChoose || (d.MinSum <= 0 && !d.HasBudget()) || len(d.Options) > oracleSumFitMax {
		return nil, false
	}
	fits := func(idx []int) bool {
		sum := 0
		for _, i := range idx {
			sum += d.Options[i].Value
		}
		if d.MinSum > 0 && sum < d.MinSum {
			return false
		}
		return !d.HasBudget() || sum <= d.MaxSum
	}
	if fits(def) {
		return nil, false
	}
	max := d.Max
	if max <= 0 || max > len(d.Options) {
		max = len(d.Options)
	}
	for size := d.Min; size <= max; size++ {
		if size < 0 {
			continue
		}
		idx := make([]int, size)
		for i := range idx {
			idx[i] = i
		}
		for {
			if fits(idx) {
				out := make([]int, size)
				for i, j := range idx {
					out[i] = d.Options[j].Index
				}
				return out, true
			}
			// Next combination in lexicographic order.
			i := size - 1
			for i >= 0 && idx[i] == len(d.Options)-size+i {
				i--
			}
			if i < 0 {
				break
			}
			idx[i]++
			for j := i + 1; j < size; j++ {
				idx[j] = idx[j-1] + 1
			}
		}
	}
	return nil, false
}

// oracleSumFitMax bounds the fallback's subset walk (2^16 subsets).
const oracleSumFitMax = 16
