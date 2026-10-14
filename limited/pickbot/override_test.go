package pickbot

import (
	"math"
	"testing"
)

func TestRatingsOverrideRescalesToSetScale(t *testing.T) {
	p := []byte(`{"Abrade":{"gih":3},"Llanowar Elves":{"gih":-1},"Nope":{"deck":1}}`)
	o, err := ParseRatingsOverride(p, "v.json", "gih", "FDN")
	if err != nil {
		t.Fatal(err)
	}
	_, sy := func() (float64, float64) {
		var ys []float64
		for _, c := range rated["FDN"].Cards {
			ys = append(ys, c.GihWr)
		}
		return meanStd(ys)
	}()
	hi, lo := o.Value("Abrade"), o.Value("Llanowar Elves")
	if math.Abs((hi-lo)-2*sy) > 1e-9 || math.Abs((hi+lo)/2-o.neutral) > 1e-9 {
		t.Fatalf("rescale wrong: hi %v lo %v neutral %v sy %v", hi, lo, o.neutral, sy)
	}
	if o.Value("Nope") != o.neutral {
		t.Fatalf("card without the field should be neutral")
	}
}

// TestRatingsOverrideColdStartScale is DR-11: a set with no embedded
// 17Lands file (BLB) parses onto the fixed fallback scale.
func TestRatingsOverrideColdStartScale(t *testing.T) {
	if _, ok := rated["BLB"]; ok {
		t.Skip("BLB now carries an embedded rating file; pick another unrated set")
	}
	p := []byte(`{"Mabel, Heir to Cragflame":{"gih":3},"Carrot Cake":{"gih":-1}}`)
	o, err := ParseRatingsOverride(p, "blb.json", "gih", "BLB")
	if err != nil {
		t.Fatal(err)
	}
	hi, lo := o.Value("Mabel, Heir to Cragflame"), o.Value("Carrot Cake")
	if math.Abs((hi-lo)-2*FallbackScaleStd) > 1e-9 || math.Abs((hi+lo)/2-FallbackScaleMean) > 1e-9 {
		t.Fatalf("fallback rescale wrong: hi %v lo %v", hi, lo)
	}
	if o.Value("Unknown") != FallbackScaleMean || len(o.Missing) != 0 {
		t.Fatalf("neutral %v missing %v", o.Value("Unknown"), o.Missing)
	}
}
