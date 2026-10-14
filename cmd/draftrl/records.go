package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/cards"
	"github.com/adams-shaun/gorge/draftlab"
	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/draftfeat"
	"github.com/adams-shaun/gorge/limited/pickbot"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// picks.bin (little-endian), version 1:
//
//	header: "DRPB" u32 version u32 G u32 C u32 nDrafters,
//	        str tableHash, nDrafters × str drafter id   (str = u16 len + bytes)
//	record: u32 pod u8 seat u8 round u8 pickNo u8 packSize
//	        u8 chosenIdx (255 = none) u8 drafterIdx u8 hasLogits u8 0
//	        G × f32 global features (globalLayout order)
//	        packSize × (i32 table index (-1 = not in table), C × f32 candidate features)
//	        if hasLogits: packSize × f32
//
// Static card features are not repeated: a reader joins the table index
// against the SetTable named by tableHash (written as table.json).
const picksBinVersion = 1

// globalLayout names the G global features, in order.
var globalLayout = func() []string {
	var out []string
	add := func(name string, n int) {
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("%s[%d]", name, i))
		}
	}
	add("seats", 1)
	add("round", 1)
	add("pick_no", 1)
	add("pack_size", 1)
	add("colour", draftfeat.NColours)
	add("mass", 1)
	add("pair_top23", draftfeat.NPairs)
	add("pair_count", draftfeat.NPairs)
	add("pair_creatures", draftfeat.NPairs)
	add("top_pairs", 2)
	add("holes", 2*draftfeat.NHoles)
	add("wheel_mass", draftfeat.NColours)
	add("have_wheel", 1)
	add("late_max", draftfeat.NColours)
	add("late_sum", draftfeat.NColours)
	add("posterior", (draftfeat.MaxSeats-1)*draftfeat.NPairs)
	add("avail", 2*draftfeat.NAvail)
	return out
}()

// candLayout names the C per-candidate features, in order.
var candLayout = []string{"v", "se", "v_top[0]", "v_top[1]", "v_max_pair", "rank", "gain[0]", "gain[1]", "wheel", "wheel_defined"}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func globalVec(f *draftfeat.Features, dst []float32) []float32 {
	dst = append(dst, float32(f.Seats), float32(f.Round), float32(f.PickNo), float32(f.PackSize))
	for _, x := range f.Colour {
		dst = append(dst, float32(x))
	}
	dst = append(dst, float32(f.Mass))
	for _, xs := range [][draftfeat.NPairs]float64{f.PairTop23, f.PairCount, f.PairCreatures} {
		for _, x := range xs {
			dst = append(dst, float32(x))
		}
	}
	dst = append(dst, float32(f.TopPairs[0]), float32(f.TopPairs[1]))
	for _, h := range f.Holes {
		for _, x := range h {
			dst = append(dst, float32(x))
		}
	}
	for _, x := range f.WheelMass {
		dst = append(dst, float32(x))
	}
	dst = append(dst, b2f(f.HaveWheel))
	for _, x := range f.LateMax {
		dst = append(dst, float32(x))
	}
	for _, x := range f.LateSum {
		dst = append(dst, float32(x))
	}
	for _, row := range f.Posterior {
		for _, x := range row {
			dst = append(dst, float32(x))
		}
	}
	for _, row := range f.Avail {
		for _, x := range row {
			dst = append(dst, float32(x))
		}
	}
	return dst
}

func candVec(c *draftfeat.Cand, dst []float32) []float32 {
	return append(dst, float32(c.V), float32(c.SE), float32(c.VTop[0]), float32(c.VTop[1]), float32(c.VMaxPair),
		float32(c.Rank), float32(c.Gain[0]), float32(c.Gain[1]), float32(c.Wheel), b2f(c.WheelDefined))
}

type binWriter struct {
	w   *bufio.Writer
	buf []byte
	vec []float32
}

func (b *binWriter) u32(x uint32) { b.buf = binary.LittleEndian.AppendUint32(b.buf, x) }
func (b *binWriter) str(s string) {
	b.buf = binary.LittleEndian.AppendUint16(b.buf, uint16(len(s)))
	b.buf = append(b.buf, s...)
}
func (b *binWriter) f32s(xs []float32) {
	for _, x := range xs {
		b.buf = binary.LittleEndian.AppendUint32(b.buf, math.Float32bits(x))
	}
}
func (b *binWriter) flush() error {
	_, err := b.w.Write(b.buf)
	b.buf = b.buf[:0]
	return err
}

// writePicksBin writes every pick record in the picks.bin format.
func writePicksBin(w io.Writer, tableHash string, drafters []string, picks []pickRec) error {
	b := &binWriter{w: bufio.NewWriter(w)}
	b.buf = append(b.buf, "DRPB"...)
	b.u32(picksBinVersion)
	b.u32(uint32(len(globalLayout)))
	b.u32(uint32(len(candLayout)))
	b.u32(uint32(len(drafters)))
	b.str(tableHash)
	for _, d := range drafters {
		b.str(d)
	}
	if err := b.flush(); err != nil {
		return err
	}
	if err := b.writeRecords(picks); err != nil {
		return err
	}
	return b.w.Flush()
}

func (b *binWriter) writeRecords(picks []pickRec) error {
	for i := range picks {
		p := &picks[i]
		if len(p.Pack) > 254 || len(p.Features.Cands) != len(p.Pack) {
			return fmt.Errorf("picks.bin: pick %d: pack %d cands %d", i, len(p.Pack), len(p.Features.Cands))
		}
		b.u32(uint32(p.Pod))
		chosen := byte(255)
		if p.ChosenIdx >= 0 {
			chosen = byte(p.ChosenIdx)
		}
		hasLogits := byte(0)
		if len(p.Logits) == len(p.Pack) && len(p.Logits) > 0 {
			hasLogits = 1
		}
		b.buf = append(b.buf, byte(p.Seat), byte(p.Round), byte(p.PickNo), byte(len(p.Pack)), chosen, byte(p.drafterIdx), hasLogits, 0)
		b.vec = globalVec(&p.Features, b.vec[:0])
		b.f32s(b.vec)
		for ci := range p.Features.Cands {
			b.u32(uint32(p.packIdx[ci]))
			b.vec = candVec(&p.Features.Cands[ci], b.vec[:0])
			b.f32s(b.vec)
		}
		if hasLogits == 1 {
			b.f32s(p.Logits)
		}
		if err := b.flush(); err != nil {
			return err
		}
	}
	return nil
}

// defaultSetTable builds the rollout's feature table when no -table is
// given: one row per card of the set's booster sheets with its printed
// facts (draftfeat.CardFromFacts) and V the z-scored builder value (the
// ratings override when the builder carries one, else pickbot.Rating), VPair
// = V, SE 0. It is a stand-in until DR-10's values table exists.
func defaultSetTable(lab *draftlab.Lab, set string, coll setdata.Collation, ov *pickbot.RatingsOverride) (*draftfeat.SetTable, error) {
	spec, err := setdata.LoadCollation(set, coll)
	if err != nil {
		return nil, err
	}
	rarity := map[string]limited.Rarity{}
	for _, sheet := range spec.Config.Sheets {
		for _, sc := range sheet.Cards {
			if r, ok := rarity[sc.Name]; !ok || sc.Rarity < r {
				rarity[sc.Name] = sc.Rarity
			}
		}
	}
	names := make([]string, 0, len(rarity))
	for n := range rarity {
		names = append(names, n)
	}
	sort.Strings(names)
	lookup := pickbot.CardLookup(func(name string) *cards.Card {
		c, _ := lab.Card(name)
		return c
	})
	rows := make([]draftfeat.Card, 0, len(names))
	vals := make([]float64, 0, len(names))
	for _, n := range names {
		c, ok := lab.Card(n)
		if !ok {
			continue
		}
		row := draftfeat.CardFromFacts(c, rarity[n])
		row.Name = n
		var v float64
		if ov != nil {
			v = ov.Value(n)
		} else {
			v = pickbot.Rating(set, limited.CardRef{Name: n, Set: set, Rarity: rarity[n]}, lookup)
		}
		rows = append(rows, row)
		vals = append(vals, v)
	}
	m, s := 0.0, 0.0
	for _, v := range vals {
		m += v
	}
	m /= float64(len(vals))
	for _, v := range vals {
		s += (v - m) * (v - m)
	}
	s = math.Sqrt(s / float64(len(vals)))
	if s == 0 {
		s = 1
	}
	for i := range rows {
		rows[i].V = (vals[i] - m) / s
		for a := range rows[i].VPair {
			rows[i].VPair[a] = rows[i].V
		}
	}
	return draftfeat.NewSetTable(set, rows, draftfeat.DefaultModel(), nil)
}

// deckHash is a deck's card-list hash: sha256 over its sorted
// "count name" lines (basics included), the first 16 hex digits. Two decks
// with the same list hash equal whatever their names.
func deckHash(d draftlab.Deck) string {
	counts := map[string]int{}
	for _, e := range d.File.Cards {
		counts[e.Name] += e.Count
	}
	lines := make([]string, 0, len(counts))
	for n, c := range counts {
		lines = append(lines, fmt.Sprintf("%d %s", c, n))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}

// gateDeckGuard holds the fdn-limited gauntlet's card-list hashes: the
// operator ruling keeps 17Lands decks out of rollouts (evaluation only), so
// every deck a rollout plays is checked against it.
type gateDeckGuard map[string]string

func newGateDeckGuard(lab *draftlab.Lab) (gateDeckGuard, error) {
	cat, err := lab.CatalogDecks("fdn-limited")
	if err != nil {
		return nil, err
	}
	g := gateDeckGuard{}
	for _, d := range cat {
		g[deckHash(d)] = d.Name
	}
	return g, nil
}

func (g gateDeckGuard) check(d draftlab.Deck) error {
	if name, ok := g[deckHash(d)]; ok {
		return fmt.Errorf("deck %s has the card list of fdn-limited gauntlet deck %q: 17Lands decks never enter rollouts", d.Name, name)
	}
	return nil
}

func writeFile(path string, fn func(w io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	if err := fn(bw); err != nil {
		f.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writePickRecords appends pick records (no header) in the picks.bin
// record format, for a writer that streams pods after the header.
func writePickRecords(w io.Writer, picks []pickRec) error {
	b := &binWriter{w: bufio.NewWriter(w)}
	if err := b.writeRecords(picks); err != nil {
		return err
	}
	return b.w.Flush()
}
