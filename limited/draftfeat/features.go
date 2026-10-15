package draftfeat

import (
	"math"

	"github.com/adams-shaun/gorge/limited/draft"
)

// Deck-shape targets for the P2 holes (spec §4.2): a starting heuristic the
// network learns weights on.
const (
	MainSize      = 23
	targetCreat   = 15
	targetTwo     = 4
	targetThree   = 4
	targetRemoval = 3
	maxTopEnd     = 3 // MV 6+ beyond this is excess
)

// Hole indices of Features.Holes.
const (
	HoleCreatures = iota
	HoleTwoDrops
	HoleThreeDrops
	HoleRemoval
	HoleTopEnd
	NHoles
)

// NAvail is one direction's availability vector: 5 colours then 10 pairs.
const NAvail = NColours + NPairs

// Features is one pick's feature set. Seat rows are RELATIVE: Posterior[o-1]
// is the seat o places to the left, (s+o) mod n, since the view carries no
// seat index; rows o >= Seats are zero.
type Features struct {
	Seats, Round, PickNo, PackSize int

	// P1: pool colour commitment (normalised shares, and their total mass)
	// and per-pair top-23 sum, on-pair playable count, on-pair creatures.
	Colour        [NColours]float64
	Mass          float64
	PairTop23     [NPairs]float64
	PairCount     [NPairs]float64
	PairCreatures [NPairs]float64
	// TopPairs are the two best pairs by PairTop23 (ties to the lower index).
	TopPairs [2]int
	// P2: deck-shape deficits for TopPairs.
	Holes [2][NHoles]float64

	// T1: the colour mass of the most recent wheel's removed set.
	WheelMass [NColours]float64
	HaveWheel bool
	// T2: per colour, max and sum of v − E_k over the latest first
	// sighting's visible cards.
	LateMax, LateSum [NColours]float64
	// T3: seat posteriors over Pairs.
	Posterior [MaxSeats - 1][NPairs]float64
	// T4: availability for the current direction [0] and next round's [1].
	Avail [2][NAvail]float64

	// Cands are group S plus T5, one per Pack card, in Pack order.
	Cands []Cand
}

// Cand is one candidate's strength features.
type Cand struct {
	ID       string
	V, SE    float64
	VTop     [2]float64 // v_{i,a} for TopPairs
	VMaxPair float64
	Rank     int        // 0 = highest v in the pack (ties by pack order)
	Gain     [2]float64 // marginal gain to TopPairs' top-23 sums
	// Wheel is T5's probability the card comes back; WheelDefined is false
	// when the pack is too small to wheel (|Pack| <= n).
	Wheel        float64
	WheelDefined bool
}

// Extract computes the features of one pick view. It is the package's only
// feature entry point and reads nothing but v and t.
func Extract(v draft.PickView, t *SetTable) Features {
	var e Extractor
	return e.Extract(v, t)
}

// Extractor is Extract with reusable scratch; its zero value is ready. It
// holds no state that survives between calls beyond buffer capacity.
type Extractor struct {
	t     *SetTable
	flat  []int32 // table index of every visible card, pack by pack
	start []int   // pack p is flat[start[p]:start[p+1]]
	picks []int32 // table index of each own pick
	round []int32 // per pack: round
	pick  []int32 // per pack: pick number within its round
	q     [MaxSeats - 1][NPairs]float64
	w     wheelScratch
	tmp   []int32

	poolStats [NPairs]pairStats

	// Exact joint posterior over the other seats' pairs, kept when the pod
	// has at most jointSeats other seats (10^3 tuples): the per-seat
	// marginals alone cannot tell which neighbour took which card of a
	// wheel, and filtering them pick by pick locks in swaps.
	joint bool
	jn    int
	jp    [jointSize]float64
}

func (e *Extractor) card(i int32) (v float64, share *[NColours]float64, fit *[NPairs]float64) {
	if i < 0 {
		return e.t.Model.VMissing, &zeroShare, &missingFit
	}
	return e.t.Cards[i].V, &e.t.share[i], &e.t.fit[i]
}

var (
	zeroShare  [NColours]float64
	missingFit = fitRow(0)
)

// Extract computes v's features against t.
func (e *Extractor) Extract(v draft.PickView, t *SetTable) Features {
	var f Features
	e.ExtractInto(&f, v, t)
	return f
}

// ExtractInto is Extract writing into f, reusing f.Cands' capacity, so a
// caller that keeps one Features per seat allocates nothing once warm.
func (e *Extractor) ExtractInto(f *Features, v draft.PickView, t *SetTable) {
	e.t = t
	n := v.Seats
	if n < 2 || n > MaxSeats {
		n = draft.Seats
	}
	*f = Features{Seats: n, Round: v.Round, PickNo: v.PickNo, PackSize: len(v.Pack), Cands: f.Cands[:0]}
	e.resolve(v)
	e.pool(f, v)
	e.table(f, v, n)
	e.cands(f, v, n)
}

// resolve maps every visible card to its table row and reconstructs each
// visible pack's round and pick number: a round lasts exactly as many picks
// as its first pack has cards.
func (e *Extractor) resolve(v draft.PickView) {
	e.flat = e.flat[:0]
	e.start = e.start[:0]
	e.round = e.round[:0]
	e.pick = e.pick[:0]
	np := len(v.Seen) + 1
	rd, rstart, rsize := int32(1), 0, -1
	for p := 0; p < np; p++ {
		pack := v.Pack
		if p < len(v.Seen) {
			pack = v.Seen[p]
		}
		if rsize >= 0 && p-rstart >= rsize {
			rd++
			rstart, rsize = p, -1
		}
		if rsize < 0 {
			rsize = len(pack)
		}
		e.start = append(e.start, len(e.flat))
		for _, c := range pack {
			e.flat = append(e.flat, int32(e.t.Index(c.Name)))
		}
		e.round = append(e.round, rd)
		e.pick = append(e.pick, int32(p-rstart+1))
	}
	e.start = append(e.start, len(e.flat))
	// The current view's own numbers are authoritative.
	if v.Round > 0 {
		e.round[np-1] = int32(v.Round)
	}
	if v.PickNo > 0 {
		e.pick[np-1] = int32(v.PickNo)
	}
	e.picks = e.picks[:0]
	for _, c := range v.Picks {
		e.picks = append(e.picks, int32(e.t.Index(c.Name)))
	}
}

func (e *Extractor) packIdx(p int) []int32 { return e.flat[e.start[p]:e.start[p+1]] }

// pool fills group P.
func (e *Extractor) pool(f *Features, _ draft.PickView) {
	t := e.t
	vr := t.vRepl
	for _, ci := range e.picks {
		val, sh, _ := e.card(ci)
		w := val - vr
		if w <= 0 {
			continue
		}
		for c := 0; c < NColours; c++ {
			f.Colour[c] += w * sh[c]
		}
	}
	for c := 0; c < NColours; c++ {
		f.Mass += f.Colour[c]
	}
	if f.Mass > 0 {
		for c := 0; c < NColours; c++ {
			f.Colour[c] /= f.Mass
		}
	}
	// Pool sorted by value, descending, ties by pick order.
	order := e.sortedPool()
	var stats [NPairs]pairStats
	for a := 0; a < NPairs; a++ {
		st := &stats[a]
		for _, pi := range order {
			ci := e.picks[pi]
			if ci < 0 {
				continue
			}
			c := &t.Cards[ci]
			if c.Types&TypeLand != 0 || c.Mask&^PairMask[a] != 0 {
				continue
			}
			st.count++
			if st.count <= MainSize {
				st.sum += c.V
				st.last = c.V
			}
			if c.Types&TypeCreature != 0 {
				st.creat++
			}
			switch {
			case c.MV == 2:
				st.two++
			case c.MV == 3:
				st.three++
			case c.MV >= 6:
				st.top++
			}
			if c.Roles&RoleRemoval != 0 {
				st.removal++
			}
		}
		f.PairTop23[a] = st.sum
		f.PairCount[a] = float64(st.count)
		f.PairCreatures[a] = float64(st.creat)
	}
	f.TopPairs = topTwo(&f.PairTop23)
	for i, a := range f.TopPairs {
		st := &stats[a]
		f.Holes[i] = [NHoles]float64{
			deficit(targetCreat, st.creat), deficit(targetTwo, st.two), deficit(targetThree, st.three),
			deficit(targetRemoval, st.removal), math.Max(0, float64(st.top-maxTopEnd)),
		}
	}
	e.poolStats = stats
}

type pairStats struct {
	sum, last                              float64
	count, creat, two, three, top, removal int
}

func deficit(target, n int) float64 { return math.Max(0, float64(target-n)) }

func topTwo(x *[NPairs]float64) [2]int {
	b0, b1 := 0, -1
	for a := 1; a < NPairs; a++ {
		if x[a] > x[b0] {
			b0, b1 = a, b0
		} else if b1 < 0 || x[a] > x[b1] {
			b1 = a
		}
	}
	return [2]int{b0, b1}
}

// sortedPool returns pick positions sorted by value descending (stable).
func (e *Extractor) sortedPool() []int32 {
	e.tmp = e.tmp[:0]
	for i := range e.picks {
		e.tmp = append(e.tmp, int32(i))
	}
	o := e.tmp
	for i := 1; i < len(o); i++ { // insertion sort: pools are ≤ 45 cards
		x := o[i]
		xv, _, _ := e.card(e.picks[x])
		j := i
		for j > 0 {
			pv, _, _ := e.card(e.picks[o[j-1]])
			if pv >= xv {
				break
			}
			o[j] = o[j-1]
			j--
		}
		o[j] = x
	}
	return o
}

// cands fills group S and T5.
func (e *Extractor) cands(f *Features, v draft.PickView, n int) {
	cur := len(v.Seen)
	idx := e.packIdx(cur)
	if f.Cands == nil || cap(f.Cands) < len(idx) {
		f.Cands = make([]Cand, len(idx))
	}
	f.Cands = f.Cands[:len(idx)]
	for i := range f.Cands {
		f.Cands[i] = Cand{}
	}
	d := direction(int(e.round[cur]))
	k := int(e.pick[cur])
	for i, ci := range idx {
		c := &f.Cands[i]
		c.ID = v.Pack[i].ID
		val, _, _ := e.card(ci)
		c.V = val
		if ci >= 0 {
			row := &e.t.Cards[ci]
			c.SE = row.SE
			c.VMaxPair = row.VPair[0]
			for a := 1; a < NPairs; a++ {
				c.VMaxPair = math.Max(c.VMaxPair, row.VPair[a])
			}
			for j, a := range f.TopPairs {
				c.VTop[j] = row.VPair[a]
				c.Gain[j] = e.gain(ci, a)
			}
		} else {
			c.VMaxPair = val
			c.VTop = [2]float64{val, val}
		}
		for j, cj := range idx {
			vj, _, _ := e.card(cj)
			if vj > val || (vj == val && j < i) {
				c.Rank++
			}
		}
	}
	if len(idx) <= n {
		return
	}
	// T5: Π_{m=1}^{n-1} (1 − π_{s+m·d}(c | Pack, q)), the pack held fixed.
	var den [MaxSeats - 1][NPairs]float64
	for m := 1; m < n; m++ {
		kap := e.kappa(int(e.round[cur]), k+m)
		for a := 0; a < NPairs; a++ {
			s := 0.0
			for _, cj := range idx {
				s += e.score(cj, a, kap)
			}
			den[m-1][a] = s
		}
	}
	for i, ci := range idx {
		p := 1.0
		for m := 1; m < n; m++ {
			o := mod(m*d, n)
			kap := e.kappa(int(e.round[cur]), k+m)
			take := 0.0
			for a := 0; a < NPairs; a++ {
				take += e.q[o-1][a] * e.score(ci, a, kap) / den[m-1][a]
			}
			p *= 1 - take
		}
		f.Cands[i].Wheel = p
		f.Cands[i].WheelDefined = true
	}
}

// gain is the marginal gain to pair a's top-23 sum if card ci is taken.
func (e *Extractor) gain(ci int32, a int) float64 {
	c := &e.t.Cards[ci]
	if c.Types&TypeLand != 0 || c.Mask&^PairMask[a] != 0 {
		return 0
	}
	st := &e.poolStats[a]
	if st.count < MainSize {
		return c.V
	}
	return math.Max(0, c.V-st.last)
}

func direction(round int) int {
	if round%2 == 0 {
		return -1
	}
	return 1
}

func mod(x, n int) int { return ((x % n) + n) % n }

// kappa is κ at pick k of the given round: a linear ramp over the first
// KappaRamp picks of the draft (round 1), full strength after.
func (e *Extractor) kappa(round, k int) float64 {
	m := &e.t.Model
	if round > 1 || m.KappaRamp <= 0 || k > m.KappaRamp {
		return m.Kappa
	}
	return m.Kappa * float64(k-1) / float64(m.KappaRamp)
}

// score is exp(β·(v_c + κ·fit(c,a))).
func (e *Extractor) score(ci int32, a int, kap float64) float64 {
	v, _, fit := e.card(ci)
	return math.Exp(e.t.Model.Beta * (v + kap*fit[a]))
}

func (e *Extractor) mk(k int) float64 {
	m := e.t.Model.M
	if k-1 < len(m) {
		return m[k-1]
	}
	return m[len(m)-1]
}
