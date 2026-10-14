package draftnet

import (
	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draft"
	"github.com/adams-shaun/gorge/limited/draftfeat"
)

// Input widths and token limits. Each vector's layout is documented on the
// filler below; the Python trainer (draftrl/py/model.py) mirrors these
// constants and the Fill layout exactly.
const (
	// FactDim: colour mask bits (5), pips/3 (5), MV/8, type bits (8),
	// rarity one-hot C/U/R/M (4), role bits (3), v, se, v_{·,a} (10).
	FactDim = 38
	// CandDim: v, se, v_top (2), max_a v_{i,a}, rank/|pack|, gain (2),
	// wheel probability, wheel-defined flag.
	CandDim = 10
	// SeatDim: directly-upstream flag, q_j (10), colour marginals (5).
	SeatDim = 16
	// CtxDim: round/3, k/P, n/8, d_r, colour shares (5), mass,
	// pair top-23/23 (10), pair count/23 (10), pair creatures/15 (10),
	// holes (2×5), wheel mass (5), have-wheel, lateness max (5), sum (5),
	// availability (2×15).
	CtxDim = 96

	MaxOthers = draftfeat.MaxSeats - 1
	MaxPool   = 45
	MaxCand   = 16
)

// Card is one card token's input: its set-table row (the embedding index,
// -1 for a card the table lacks) and its facts vector.
type Card struct {
	Emb   int32
	Facts [FactDim]float32
}

// PoolTok is one own pick: the card and its 0-based pick ordinal.
type PoolTok struct {
	Card
	Pick int32
}

// CandTok is one pack card on offer.
type CandTok struct {
	Card
	Feat [CandDim]float32
}

// Inputs is one pick's token set. Its slices are reused across Fill calls.
type Inputs struct {
	Ctx     [CtxDim]float32
	NSeat   int
	SeatOff [MaxOthers]int // relative offset o-1 of the seat (s+o) mod n
	Seat    [MaxOthers][SeatDim]float32
	Pool    []PoolTok
	Cand    []CandTok
}

// Fill encodes one pick's features into in. f must be draftfeat's features
// of v against t. The candidates follow v.Pack's order. It allocates only
// to grow in's slices.
func (in *Inputs) Fill(f *draftfeat.Features, v draft.PickView, t *draftfeat.SetTable) {
	n := f.Seats
	d := 1
	if f.Round%2 == 0 {
		d = -1
	}
	c := &in.Ctx
	*c = [CtxDim]float32{}
	P := f.PickNo + f.PackSize - 1
	if P < 1 {
		P = 1
	}
	c[0] = float32(f.Round) / 3
	c[1] = float32(f.PickNo) / float32(P)
	c[2] = float32(n) / 8
	c[3] = float32(d)
	i := 4
	put := func(x float64) { c[i] = float32(x); i++ }
	for _, x := range f.Colour {
		put(x)
	}
	put(f.Mass)
	for _, x := range f.PairTop23 {
		put(x / 23)
	}
	for _, x := range f.PairCount {
		put(x / 23)
	}
	for _, x := range f.PairCreatures {
		put(x / 15)
	}
	for _, h := range f.Holes {
		for _, x := range h {
			put(x)
		}
	}
	for _, x := range f.WheelMass {
		put(x)
	}
	put(b2f(f.HaveWheel))
	for _, x := range f.LateMax {
		put(x)
	}
	for _, x := range f.LateSum {
		put(x)
	}
	for _, a := range f.Avail {
		for _, x := range a {
			put(x)
		}
	}

	// Seats: offset o is seat (s+o) mod n; the seat passing to us this
	// round is o = -d mod n.
	in.NSeat = n - 1
	up := ((-d)%n + n) % n
	for o := 1; o < n; o++ {
		s := &in.Seat[o-1]
		*s = [SeatDim]float32{}
		in.SeatOff[o-1] = o - 1
		if o == up {
			s[0] = 1
		}
		q := &f.Posterior[o-1]
		for a, p := range q {
			s[1+a] = float32(p)
			for _, col := range draftfeat.Pairs[a] {
				s[1+draftfeat.NPairs+col] += float32(p)
			}
		}
	}

	in.Pool = in.Pool[:0]
	for k, ref := range v.Picks {
		if len(in.Pool) == MaxPool {
			break
		}
		in.Pool = append(in.Pool, PoolTok{Pick: int32(k)})
		fillCard(&in.Pool[len(in.Pool)-1].Card, t, ref)
	}
	in.Cand = in.Cand[:0]
	for j, ref := range v.Pack {
		if j == MaxCand {
			break
		}
		in.Cand = append(in.Cand, CandTok{})
		ct := &in.Cand[len(in.Cand)-1]
		fillCard(&ct.Card, t, ref)
		if j >= len(f.Cands) {
			continue
		}
		fc := &f.Cands[j]
		ct.Feat = [CandDim]float32{
			float32(fc.V), float32(fc.SE), float32(fc.VTop[0]), float32(fc.VTop[1]), float32(fc.VMaxPair),
			float32(fc.Rank) / float32(max(1, len(v.Pack))), float32(fc.Gain[0]), float32(fc.Gain[1]),
			float32(fc.Wheel), float32(b2f(fc.WheelDefined)),
		}
	}
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// fillCard encodes ref's set-table row; a card the table lacks is all zero
// with embedding -1.
func fillCard(c *Card, t *draftfeat.SetTable, ref limited.CardRef) {
	c.Facts = [FactDim]float32{}
	idx := t.Index(ref.Name)
	c.Emb = int32(idx)
	if idx < 0 {
		return
	}
	row := &t.Cards[idx]
	f := &c.Facts
	for b := 0; b < 5; b++ {
		if row.Mask&(1<<b) != 0 {
			f[b] = 1
		}
		f[5+b] = float32(row.Pips[b]) / 3
	}
	f[10] = float32(row.MV) / 8
	for b := 0; b < 8; b++ {
		if row.Types&(1<<b) != 0 {
			f[11+b] = 1
		}
	}
	switch row.Rarity {
	case limited.Uncommon:
		f[20] = 1
	case limited.Rare:
		f[21] = 1
	case limited.Mythic:
		f[22] = 1
	default:
		f[19] = 1
	}
	for b := 0; b < 3; b++ {
		if row.Roles&(1<<b) != 0 {
			f[23+b] = 1
		}
	}
	f[26] = float32(row.V)
	f[27] = float32(row.SE)
	for a, x := range row.VPair {
		f[28+a] = float32(x)
	}
}
