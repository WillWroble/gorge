package pickbot

import (
	"math"
	"math/rand/v2"
	"sync"

	"github.com/adams-shaun/gorge/limited/draftfeat"
	"github.com/adams-shaun/gorge/limited/draftnet"
)

// Recorder receives every NetDrafter decision: the view, its draftfeat
// features, the encoded network inputs, the candidate logits (in v.Pack
// order) and the chosen pack index. Everything it is handed is scratch the
// drafter reuses after Record returns, so a recorder copies what it keeps.
// Rollouts (DR-7) and behaviour cloning (DR-8) hang off it.
type Recorder interface {
	Record(v PickView, f *draftfeat.Features, in *draftnet.Inputs, logits []float32, chosen int)
}

// NetDrafter picks with the draft-RL policy network (draft-RL spec §5). It
// is stateless across picks: every input is recomputed from the PickView,
// the only legal view a seat has, and it can only return an ID from
// v.Pack. Temperature 0 is hosted play (argmax, seeded tie-break);
// Temperature > 0 samples softmax(logits/T) with the seat rng, as training
// rollouts do at T = 1.
type NetDrafter struct {
	Net         *draftnet.Net
	Table       *draftfeat.SetTable
	Temperature float64
	Recorder    Recorder
}

// netScratch is one pick's reusable buffers. It carries nothing from one
// pick to the next that changes a result.
type netScratch struct {
	ex draftfeat.Extractor
	f  draftfeat.Features
	in draftnet.Inputs
	r  draftnet.Runner
}

var netScratchPool = sync.Pool{New: func() any { return new(netScratch) }}

// Pick returns the chosen card's ID, or "" for an empty pack. A network
// failure (no candidates encodable) falls back to the first pack card so a
// pod never stalls.
func (d *NetDrafter) Pick(v PickView, r *rand.Rand) string {
	if len(v.Pack) == 0 {
		return ""
	}
	if d.Net == nil || d.Table == nil {
		return v.Pack[0].ID
	}
	s := netScratchPool.Get().(*netScratch)
	defer netScratchPool.Put(s)
	s.ex.ExtractInto(&s.f, v, d.Table)
	f := &s.f
	s.in.Fill(f, v, d.Table)
	logits, err := s.r.Forward(d.Net, &s.in)
	if err != nil {
		return v.Pack[0].ID
	}
	var chosen int
	if d.Temperature > 0 && r != nil {
		chosen = sampleLogits(logits, d.Temperature, r)
	} else {
		chosen = argmaxLogits(logits, r)
	}
	if d.Recorder != nil {
		d.Recorder.Record(v, f, &s.in, logits, chosen)
	}
	return v.Pack[chosen].ID
}

// argmaxLogits is the highest logit, an exact tie broken uniformly with r
// (the lowest index when r is nil). r is drawn only on a tie.
func argmaxLogits(l []float32, r *rand.Rand) int {
	best, ties := 0, 1
	for i := 1; i < len(l); i++ {
		switch {
		case l[i] > l[best]:
			best, ties = i, 1
		case l[i] == l[best]:
			ties++
		}
	}
	if ties == 1 || r == nil {
		return best
	}
	k := r.IntN(ties)
	for i := range l {
		if l[i] == l[best] {
			if k == 0 {
				return i
			}
			k--
		}
	}
	return best
}

// sampleLogits draws from softmax(l/temp) with one r.Float64 draw; the sum
// runs in index order.
func sampleLogits(l []float32, temp float64, r *rand.Rand) int {
	mx := float64(l[0])
	for _, x := range l[1:] {
		mx = math.Max(mx, float64(x))
	}
	var sum float64
	for _, x := range l {
		sum += math.Exp((float64(x) - mx) / temp)
	}
	u := r.Float64() * sum
	for i, x := range l {
		u -= math.Exp((float64(x) - mx) / temp)
		if u < 0 {
			return i
		}
	}
	return len(l) - 1
}
