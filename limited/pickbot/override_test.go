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
	if _, err := ParseRatingsOverride(p, "v.json", "gih", "XXX"); err == nil {
		t.Fatal("unrated set should error")
	}
}
