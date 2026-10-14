package pickbot

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// RatingsOverride is a training-only replacement for the embedded rating
// file: per-card values from an external JSON file (cmd/cardval output,
// {"Card": {"gih": x, ...}}), affinely rescaled onto the set's 17Lands
// gih_wr scale (same mean and population std) so the drafter's tuning
// constants keep their meaning. A card the file lacks gets the 17Lands
// mean, i.e. a centred-neutral value; it never falls back to 17Lands.
type RatingsOverride struct {
	values  map[string]float64
	neutral float64
	// Missing counts set cards rated by 17Lands but absent from the file.
	Missing []string
}

// Value is the card's rescaled rating (a gih_wr-equivalent win rate).
func (o *RatingsOverride) Value(name string) float64 {
	if v, ok := o.values[name]; ok {
		return v
	}
	return o.neutral
}

func meanStd(xs []float64) (float64, float64) {
	m := 0.0
	for _, x := range xs {
		m += x
	}
	m /= float64(len(xs))
	v := 0.0
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return m, math.Sqrt(v / float64(len(xs)))
}

// ParseRatingsOverride takes field from every card entry of the JSON in b
// (read from path by the caller; path only labels errors) and rescales it
// onto set's embedded 17Lands scale. The package stays pure: no file I/O.
func ParseRatingsOverride(b []byte, path, field, set string) (*RatingsOverride, error) {
	ref, ok := rated[set]
	if !ok {
		return nil, fmt.Errorf("ratings override: set %s has no embedded rating file to take the scale from", set)
	}
	var raw map[string]map[string]float64
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("ratings override %s: %w", path, err)
	}
	names := make([]string, 0, len(raw))
	for n, e := range raw {
		if _, ok := e[field]; ok {
			names = append(names, n)
		}
	}
	if len(names) < 2 {
		return nil, fmt.Errorf("ratings override %s: fewer than 2 cards carry field %q", path, field)
	}
	sort.Strings(names)
	xs := make([]float64, len(names))
	for i, n := range names {
		xs[i] = raw[n][field]
	}
	refNames := make([]string, 0, len(ref.Cards))
	for n := range ref.Cards {
		refNames = append(refNames, n)
	}
	sort.Strings(refNames)
	ys := make([]float64, len(refNames))
	for i, n := range refNames {
		ys[i] = ref.Cards[n].GihWr
	}
	mx, sx := meanStd(xs)
	my, sy := meanStd(ys)
	if sx == 0 {
		return nil, fmt.Errorf("ratings override %s: field %q has zero spread", path, field)
	}
	o := &RatingsOverride{values: make(map[string]float64, len(names)), neutral: my}
	for i, n := range names {
		o.values[n] = my + (xs[i]-mx)*sy/sx
	}
	for _, n := range refNames {
		if _, ok := o.values[n]; !ok {
			o.Missing = append(o.Missing, n)
		}
	}
	return o, nil
}
