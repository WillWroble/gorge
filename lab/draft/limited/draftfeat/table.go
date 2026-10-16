// Package draftfeat turns one seat's legal draft view into the pick
// features of the draft-RL design (docs/design/
// 2026-10-14-draft-rl-design.md §4): pool signals (group P), per-candidate
// card strength (group S) and table reads (group T: wheels, lateness, seat
// posteriors, availability, wheel probability).
//
// Extract reads only a draft.PickView — what the seat legally saw — and a
// SetTable of static, public set data. It never sees another seat's picks,
// the pod or any per-event state, and it keeps no state between picks:
// every table read is recomputed from Seen at each pick (spec D1).
//
// The package is pure (spec §2.3): no clock, no I/O, no legacy math/rand,
// and float reductions run in a fixed order, so equal inputs give
// bit-equal features.
package draftfeat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/lab/draft/limited"
)

// NPairs is the number of two-colour pairs; NColours the WUBRG colours;
// MaxSeats the largest pod draft.NewPod accepts.
const (
	NPairs   = 10
	NColours = 5
	MaxSeats = 8
)

// Pairs is the ten colour pairs in fixed order (WUBRG bit indices), the same
// order lab/draft/limited/pickbot's builder scans, so a pair index means one thing
// across the draft code.
var Pairs = [NPairs][2]int{
	{0, 1}, {0, 2}, {0, 3}, {0, 4},
	{1, 2}, {1, 3}, {1, 4},
	{2, 3}, {2, 4},
	{3, 4},
}

// PairMask is Pairs as WUBRG bitmasks.
var PairMask = func() (m [NPairs]uint8) {
	for a, p := range Pairs {
		m[a] = 1<<p[0] | 1<<p[1]
	}
	return
}()

// Type bits of Card.Types.
const (
	TypeCreature uint8 = 1 << iota
	TypeInstant
	TypeSorcery
	TypeLand
	TypeArtifact
	TypeEnchantment
	TypePlaneswalker
	TypeBasic
)

// Role bits of Card.Roles.
const (
	// RoleRemoval is pickbot.Heuristic's oracle rule: an instant or sorcery
	// that says "destroy target", "exile target" or "damage to target".
	RoleRemoval uint8 = 1 << iota
	// RoleEvasive is a creature with an evasion keyword.
	RoleEvasive
	// RoleFixing is a nonbasic land of two or more colours.
	RoleFixing
)

// Card is one card's static row: printed facts plus prior values. V is the
// centred prior value (a z-score), SE its standard error, VPair the
// pair-conditional value per Pairs entry.
type Card struct {
	Name   string          `json:"name"`
	Mask   uint8           `json:"mask"`
	Pips   [NColours]uint8 `json:"pips"`
	MV     uint8           `json:"mv"`
	Types  uint8           `json:"types"`
	Rarity limited.Rarity  `json:"rarity"`
	Roles  uint8           `json:"roles"`
	V      float64         `json:"v"`
	SE     float64         `json:"se"`
	VPair  [NPairs]float64 `json:"v_pair"`
}

// Model is the opponent pick model's constants (spec §4.4 T3/T4):
// π(c | pack, a) ∝ exp(β·(v_c + κ_k·fit(c,a))), κ_k ramping linearly from 0
// to Kappa over the first KappaRamp picks of the draft; M[k-1] is the
// phantom "card not in V" score m_k of the rejection likelihood (the last
// entry holds for later picks); Tau the availability take rate; Carry the
// weight a posterior keeps across a round boundary (the rest goes to
// uniform); VMissing the value of a card the table does not list. DR-4's
// calibrate fits them; DefaultModel is a starting point.
type Model struct {
	Beta      float64   `json:"beta"`
	Kappa     float64   `json:"kappa"`
	KappaRamp int       `json:"kappa_ramp"`
	M         []float64 `json:"m"`
	Tau       float64   `json:"tau"`
	Carry     float64   `json:"carry"`
	VMissing  float64   `json:"v_missing"`
}

// DefaultModel is the uncalibrated opponent model.
func DefaultModel() Model {
	return Model{Beta: 2, Kappa: 1.5, KappaRamp: 5, M: []float64{1}, Tau: 0.5, Carry: 0.9, VMissing: -3}
}

// SetTable is one set's static, public data: card rows sorted by name, the
// opponent model, and Expected[k-1] = E_k, the expected value of a card
// still in a pack at pick k (from S0 simulations). It never holds
// per-event data. Build one with NewSetTable or DecodeSetTable; the zero
// value is not usable.
type SetTable struct {
	Set      string    `json:"set"`
	Cards    []Card    `json:"cards"`
	Model    Model     `json:"model"`
	Expected []float64 `json:"expected"`

	hash  string
	vRepl float64
	share [][NColours]float64 // pip share per card
	fit   [][NPairs]float64   // fit(c, a) per card
}

// NewSetTable validates and indexes a table: rows are sorted by name and a
// duplicate name is an error. A zero Rarity reads as Common.
func NewSetTable(set string, rows []Card, m Model, expected []float64) (*SetTable, error) {
	t := &SetTable{Set: set, Cards: append([]Card(nil), rows...), Model: m, Expected: append([]float64(nil), expected...)}
	if err := t.finish(); err != nil {
		return nil, err
	}
	return t, nil
}

// DecodeSetTable reads a table from its JSON form.
func DecodeSetTable(b []byte) (*SetTable, error) {
	var t SetTable
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("draftfeat: decode set table: %w", err)
	}
	if err := t.finish(); err != nil {
		return nil, err
	}
	return &t, nil
}

// Encode is the table's canonical JSON form; DecodeSetTable(Encode())
// gives an equal table with the same Hash.
func (t *SetTable) Encode() ([]byte, error) { return json.Marshal(t) }

// Hash is the hex SHA-256 prefix of the canonical encoding: the table's
// version, recorded beside anything trained or rolled out on it.
func (t *SetTable) Hash() string { return t.hash }

// VRepl is the replacement level: the 25th-percentile value of the
// table's playable (nonland) cards.
func (t *SetTable) VRepl() float64 { return t.vRepl }

// Index returns name's row, or -1. It is a binary search over the sorted
// rows, so lookups allocate nothing.
func (t *SetTable) Index(name string) int {
	i := sort.Search(len(t.Cards), func(i int) bool { return t.Cards[i].Name >= name })
	if i < len(t.Cards) && t.Cards[i].Name == name {
		return i
	}
	return -1
}

func (t *SetTable) finish() error {
	m := &t.Model
	if m.Beta <= 0 || math.IsNaN(m.Beta) {
		return errors.New("draftfeat: model beta must be > 0")
	}
	if len(m.M) == 0 {
		return errors.New("draftfeat: model needs at least one m_k")
	}
	if m.Carry < 0 || m.Carry > 1 {
		return errors.New("draftfeat: model carry must be in [0,1]")
	}
	sort.SliceStable(t.Cards, func(i, j int) bool { return t.Cards[i].Name < t.Cards[j].Name })
	for i := range t.Cards {
		if t.Cards[i].Name == "" {
			return errors.New("draftfeat: card row with no name")
		}
		if i > 0 && t.Cards[i].Name == t.Cards[i-1].Name {
			return fmt.Errorf("draftfeat: duplicate card %q", t.Cards[i].Name)
		}
		if t.Cards[i].Rarity == 0 {
			t.Cards[i].Rarity = limited.Common
		}
	}
	t.share = make([][NColours]float64, len(t.Cards))
	t.fit = make([][NPairs]float64, len(t.Cards))
	var playable []float64
	for i := range t.Cards {
		c := &t.Cards[i]
		t.share[i] = pipShare(c.Mask, c.Pips)
		t.fit[i] = fitRow(c.Mask)
		if c.Types&TypeLand == 0 {
			playable = append(playable, c.V)
		}
	}
	if len(playable) > 0 {
		sort.Float64s(playable)
		t.vRepl = playable[(len(playable)-1)/4]
	}
	b, err := t.Encode()
	if err != nil {
		return fmt.Errorf("draftfeat: encode set table: %w", err)
	}
	sum := sha256.Sum256(b)
	t.hash = hex.EncodeToString(sum[:8])
	return nil
}

// pipShare is a card's colour share: its pips normalised, or an even split
// over its colour mask when it has no coloured pips; colourless is zero.
func pipShare(mask uint8, pips [NColours]uint8) (s [NColours]float64) {
	tot := 0
	for _, p := range pips {
		tot += int(p)
	}
	if tot > 0 {
		for i, p := range pips {
			s[i] = float64(p) / float64(tot)
		}
		return
	}
	n := colourCount(mask)
	if n == 0 {
		return
	}
	for i := 0; i < NColours; i++ {
		if mask&(1<<i) != 0 {
			s[i] = 1 / float64(n)
		}
	}
	return
}

// fitRow is fit(c, a) = |col(c) ∩ a| / |col(c)| per pair; colourless 0.5.
func fitRow(mask uint8) (f [NPairs]float64) {
	n := colourCount(mask)
	for a := range f {
		if n == 0 {
			f[a] = 0.5
			continue
		}
		f[a] = float64(colourCount(mask&PairMask[a])) / float64(n)
	}
	return
}

func colourCount(m uint8) int {
	n := 0
	for i := 0; i < NColours; i++ {
		if m&(1<<i) != 0 {
			n++
		}
	}
	return n
}

var evasion = []string{"Flying", "Menace", "Shadow", "Intimidate", "Fear", "Skulk"}

// CardFromFacts fills a row's printed facts from the compiled card: colour
// identity and pips (pickbot's reading), mana value, type bits and role
// bits. Values (V, SE, VPair) are the caller's.
func CardFromFacts(c *cards.Card, r limited.Rarity) Card {
	row := Card{Rarity: r}
	if c == nil || len(c.Faces) == 0 {
		return row
	}
	f := c.Faces[0]
	row.Name = f.Name
	row.Mask = c.ColourIdentity() & 0x1f
	for _, face := range c.Faces {
		for i := 0; i < NColours; i++ {
			row.Pips[i] += uint8(cards.ManaCostChroma(face.ManaCost, 1<<i))
		}
	}
	if mv := f.Cmc(); mv > 0 {
		if mv > 255 {
			mv = 255
		}
		row.MV = uint8(mv)
	}
	for _, tb := range []struct {
		ok  bool
		bit uint8
	}{
		{f.IsCreature(), TypeCreature}, {f.IsInstant(), TypeInstant}, {f.IsSorcery(), TypeSorcery},
		{f.IsLand(), TypeLand}, {f.IsArtifact(), TypeArtifact}, {f.IsEnchantment(), TypeEnchantment},
		{f.IsPlaneswalker(), TypePlaneswalker}, {f.IsBasic(), TypeBasic},
	} {
		if tb.ok {
			row.Types |= tb.bit
		}
	}
	if f.IsInstant() || f.IsSorcery() {
		o := strings.ToLower(f.Oracle)
		if strings.Contains(o, "destroy target") || strings.Contains(o, "exile target") || strings.Contains(o, "damage to target") {
			row.Roles |= RoleRemoval
		}
	}
	if f.IsCreature() {
		for _, k := range evasion {
			if f.HasKeyword(k) {
				row.Roles |= RoleEvasive
				break
			}
		}
	}
	if f.IsLand() && !f.IsBasic() && colourCount(row.Mask) >= 2 {
		row.Roles |= RoleFixing
	}
	return row
}
