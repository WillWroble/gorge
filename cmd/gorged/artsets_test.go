//go:build lab

package main

import (
	"sort"
	"testing"
)

// TestSetCardNamesCoverTheDraftSets pins prewarmSetArt's name list: every
// embedded draft set's sheet cards, sorted and de-duplicated, so a fresh
// draft pack's art is warm before any browser asks for it.
func TestSetCardNamesCoverTheDraftSets(t *testing.T) {
	names, err := setCardNames()
	if err != nil {
		t.Fatal(err)
	}
	if !sort.StringsAreSorted(names) {
		t.Fatal("setCardNames is not sorted")
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("duplicate name %q", n)
		}
		seen[n] = true
	}
	// Cards seen blank in an FDN draft pack on the demo (2026-10-14), and one BLB card.
	for _, want := range []string{"Make Your Move", "Macabre Waltz", "Lunar Insight", "Faebloom Trick", "Mindwhisker"} {
		if !seen[want] {
			t.Errorf("setCardNames lacks %q", want)
		}
	}
	if len(names) < 500 {
		t.Errorf("setCardNames has %d names, want the FDN and BLB sheets (500+)", len(names))
	}
}
