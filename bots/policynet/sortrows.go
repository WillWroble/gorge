package policynet

import "sync"

// sortScratch is stableSortByRow's reusable copy buffer.
type sortScratch struct{ tmp []Feature }

var sortScratches = sync.Pool{New: func() any { return new(sortScratch) }}

// stableSortByRow sorts fs by Row ascending, stably: the exact permutation
// slices.SortStableFunc(fs, cmpFeatureRow) produces (a stable sort's output
// is unique). A short list is insertion-sorted; a longer one is an LSD radix
// sort, two stable counting passes over the row's low and high bytes.
func stableSortByRow(fs []Feature) {
	n := len(fs)
	if n < 2 {
		return
	}
	sorted := true
	for i := 1; i < n; i++ {
		if fs[i].Row < fs[i-1].Row {
			sorted = false
			break
		}
	}
	if sorted {
		return
	}
	if n <= 24 {
		for i := 1; i < n; i++ {
			for j := i; j > 0 && fs[j].Row < fs[j-1].Row; j-- {
				fs[j], fs[j-1] = fs[j-1], fs[j]
			}
		}
		return
	}
	sc := sortScratches.Get().(*sortScratch)
	if cap(sc.tmp) < n {
		sc.tmp = make([]Feature, n, n+n/2)
	}
	tmp := sc.tmp[:n]
	radixPass(tmp, fs, 0)
	radixPass(fs, tmp, 8)
	sortScratches.Put(sc)
}

// radixPass stably scatters src into dst by byte (Row >> shift) & 0xff.
func radixPass(dst, src []Feature, shift uint) {
	var count [256]int
	for _, f := range src {
		count[(f.Row>>shift)&0xff]++
	}
	pos := 0
	for b := range count {
		c := count[b]
		count[b] = pos
		pos += c
	}
	for _, f := range src {
		b := (f.Row >> shift) & 0xff
		dst[count[b]] = f
		count[b]++
	}
}
