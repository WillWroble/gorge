package booster_test

import (
	"math/rand/v2"
	"reflect"
	"strconv"
	"testing"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/booster"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// sheetCard builds one SheetCard.
func sheetCard(name, number string, r limited.Rarity) setdata.SheetCard {
	return setdata.SheetCard{Name: name, Number: number, Rarity: r}
}

// fiveColourSheet returns 20 W, 6 U, 6 B, 6 R and 2 G mono commons, named
// W1..W20, U1..U6, B1..B6, R1..R6, G1..G2, so a 7-common draw usually misses
// green and the colour balance has to trade a white for it.
func fiveColourSheet() []setdata.SheetCard {
	var sheet []setdata.SheetCard
	for i := 1; i <= 20; i++ {
		sheet = append(sheet, sheetCard("W"+itoa(i), itoa(i), limited.Common))
	}
	for _, col := range []byte{'U', 'B', 'R'} {
		for i := 1; i <= 6; i++ {
			sheet = append(sheet, sheetCard(string(col)+itoa(i), itoa(i), limited.Common))
		}
	}
	for i := 1; i <= 2; i++ {
		sheet = append(sheet, sheetCard("G"+itoa(i), itoa(i), limited.Common))
	}
	return sheet
}

func itoa(i int) string { return strconv.Itoa(i) }

// monoCount counts the slot cards that are mono-coloured of each WUBRG
// colour, using the test's own colourOf table.
func monoCount(colourOf booster.ColourOf, cards []limited.CardRef) [5]int {
	var out [5]int
	for _, c := range cards {
		cs := colourOf(c.Name)
		if len(cs) == 1 {
			switch cs[0] {
			case 'W':
				out[0]++
			case 'U':
				out[1]++
			case 'B':
				out[2]++
			case 'R':
				out[3]++
			case 'G':
				out[4]++
			}
		}
	}
	return out
}

// TestBoosterPackFillsSlotsInOrderAndIDs opens a hand spec whose slots are
// recognisable by rarity and checks the pack fills in spec order with
// sequential IDs from the caller's allocator.
func TestBoosterPackFillsSlotsInOrderAndIDs(t *testing.T) {
	spec := setdata.Spec{
		Code: "TST", Name: "Test", PackSize: 5, MainMax: 100,
		Sheets: map[string][]setdata.SheetCard{
			"common":   {sheetCard("C1", "1", limited.Common), sheetCard("C2", "2", limited.Common), sheetCard("C3", "3", limited.Common)},
			"uncommon": {sheetCard("U1", "10", limited.Uncommon)},
			"rare":     {sheetCard("R1", "20", limited.Rare)},
			"mythic":   {sheetCard("M1", "30", limited.Mythic)},
			"land":     {sheetCard("L1", "90", limited.Land)},
		},
		Slots: []setdata.Slot{
			{Sheet: "common", Count: 2},
			{RareMythic: &setdata.RareMythic{RareWeight: 2, MythicWeight: 1}, Count: 1},
			{Sheet: "uncommon", Count: 1},
			{Sheet: "land", Count: 1},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("precondition: hand spec is invalid: %v", err)
	}
	pack, err := booster.Open(spec, rand.New(rand.NewPCG(1, 2)), &booster.IDs{}, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(pack) != 5 {
		t.Fatalf("pack has %d cards, want 5", len(pack))
	}
	wantRarity := []limited.Rarity{limited.Common, limited.Common, limited.Rare, limited.Uncommon, limited.Land}
	for i, r := range wantRarity {
		if i == 2 && (pack[i].Rarity == limited.Mythic) {
			r = limited.Mythic // the rare/mythic slot may draw either side of the split
		}
		if pack[i].Rarity != r {
			t.Errorf("pack[%d] rarity = %c, want %c (slots must fill in spec order)", i, pack[i].Rarity, r)
		}
		if pack[i].Set != "TST" {
			t.Errorf("pack[%d].Set = %q, want TST", i, pack[i].Set)
		}
		wantID := "c" + itoa(i+1)
		if pack[i].ID != wantID {
			t.Errorf("pack[%d].ID = %q, want %q (IDs assigned in slot-fill order)", i, pack[i].ID, wantID)
		}
	}
}

// TestBoosterPackNeverRepeatsNames drives the resample rule from both
// directions: two draws from a two-card sheet come out distinct, and a third
// draw from the same two-card sheet is an error, not a duplicate.
func TestBoosterPackNeverRepeatsNames(t *testing.T) {
	twoCards := []setdata.SheetCard{sheetCard("Only1", "1", limited.Common), sheetCard("Only2", "2", limited.Common)}
	packOf := func(counts ...int) setdata.Spec {
		spec := setdata.Spec{
			Code: "TST", Name: "Test", MainMax: 10,
			Sheets: map[string][]setdata.SheetCard{"common": twoCards},
		}
		total := 0
		for _, c := range counts {
			spec.Slots = append(spec.Slots, setdata.Slot{Sheet: "common", Count: c})
			total += c
		}
		spec.PackSize = total
		return spec
	}
	pack, err := booster.Open(packOf(2), rand.New(rand.NewPCG(3, 4)), &booster.IDs{}, nil)
	if err != nil {
		t.Fatalf("Open two draws from a two-card sheet: %v", err)
	}
	if len(pack) != 2 || pack[0].Name == pack[1].Name {
		t.Fatalf("two draws gave %v: a pack must never repeat a name", pack)
	}
	if _, err := booster.Open(packOf(3), rand.New(rand.NewPCG(3, 4)), &booster.IDs{}, nil); err == nil {
		t.Error("three draws from a two-card sheet succeeded: duplicates must fail Open, not pass silently")
	}
}

// TestBoosterRareMythicSplitFollowsPerCardWeights pins the 2:1 per-card
// split: with two rares at weight 2 and one mythic at weight 1, the mythic
// takes the rare/mythic slot one time in five.
func TestBoosterRareMythicSplitFollowsPerCardWeights(t *testing.T) {
	spec := setdata.Spec{
		Code: "TST", Name: "Test", PackSize: 1, MainMax: 10,
		Sheets: map[string][]setdata.SheetCard{
			"rare":   {sheetCard("R1", "1", limited.Rare), sheetCard("R2", "2", limited.Rare)},
			"mythic": {sheetCard("M1", "3", limited.Mythic)},
		},
		Slots: []setdata.Slot{{RareMythic: &setdata.RareMythic{RareWeight: 2, MythicWeight: 1}, Count: 1}},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("precondition: hand spec is invalid: %v", err)
	}
	const packs = 20000
	mythics := 0
	for i := 0; i < packs; i++ {
		pack, err := booster.Open(spec, rand.New(rand.NewPCG(uint64(i), 7)), &booster.IDs{}, nil)
		if err != nil || len(pack) != 1 {
			t.Fatalf("Open pack %d: %v", i, err)
		}
		if pack[0].Rarity == limited.Mythic {
			mythics++
		}
	}
	share := float64(mythics) / packs // want 1*1 / (2*2 + 1*1) = 0.2
	if share < 0.18 || share > 0.22 {
		t.Errorf("mythic share of the rare/mythic slot = %.4f over %d packs, want 0.2 ± 0.02", share, packs)
	}
}

// TestBoosterWildcardRarityFollowsWeights pins a wildcard slot's rarity
// distribution on its weights and the rare_mythic bucket's internal 2:1
// per-card split (two rares at weight 2 vs one mythic at weight 1 → one
// mythic in five of the bucket's draws).
func TestBoosterWildcardRarityFollowsWeights(t *testing.T) {
	spec := setdata.Spec{
		Code: "TST", Name: "Test", PackSize: 2, MainMax: 10,
		Sheets: map[string][]setdata.SheetCard{
			"common":   {sheetCard("C1", "1", limited.Common), sheetCard("C2", "2", limited.Common), sheetCard("C3", "3", limited.Common), sheetCard("C4", "4", limited.Common)},
			"uncommon": {sheetCard("U1", "5", limited.Uncommon), sheetCard("U2", "6", limited.Uncommon), sheetCard("U3", "7", limited.Uncommon), sheetCard("U4", "8", limited.Uncommon)},
			"rare":     {sheetCard("R1", "9", limited.Rare), sheetCard("R2", "10", limited.Rare)},
			"mythic":   {sheetCard("M1", "11", limited.Mythic)},
		},
		Slots: []setdata.Slot{
			{RareMythic: &setdata.RareMythic{RareWeight: 2, MythicWeight: 1}, Count: 1},
			{Wildcard: &setdata.Weights{Common: 0.5, Uncommon: 0.4, RareMythic: 0.1}, Count: 1},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("precondition: hand spec is invalid: %v", err)
	}
	const packs = 20000
	var got [4]int // C U R M
	for i := 0; i < packs; i++ {
		pack, err := booster.Open(spec, rand.New(rand.NewPCG(uint64(i), 9)), &booster.IDs{}, nil)
		if err != nil || len(pack) != 2 {
			t.Fatalf("Open pack %d: %v", i, err)
		}
		switch pack[1].Rarity { // the wildcard draw sits after the rare/mythic slot

		case limited.Common:
			got[0]++
		case limited.Uncommon:
			got[1]++
		case limited.Rare:
			got[2]++
		case limited.Mythic:
			got[3]++
		default:
			t.Fatalf("wildcard drew rarity %c outside C/U/R/M", pack[0].Rarity)
		}
	}
	want := [4]float64{0.5, 0.4, 0.08, 0.02} // rare_mythic 0.1 splits 2:1 per card
	for i, w := range want {
		share := float64(got[i]) / packs
		if share < w-0.01 || share > w+0.01 {
			t.Errorf("wildcard rarity %c share = %.4f, want %.4f ± 0.01", "CURM"[i], share, w)
		}
	}
}

// TestBoosterColourBalanceFillsMissingColours draws 7 commons from a sheet
// where green has only two cards, so most packs miss green; the colour
// balance must trade a white (the most-represented colour) for a green in
// every one of them, without ever emptying white or duplicating a name.
func TestBoosterColourBalanceFillsMissingColours(t *testing.T) {
	sheet := fiveColourSheet()
	spec := setdata.Spec{
		Code: "TST", Name: "Test", PackSize: 7, MainMax: 40,
		Sheets: map[string][]setdata.SheetCard{"common": sheet},
		Slots:  []setdata.Slot{{Sheet: "common", Count: 7, BalanceColours: true}},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("precondition: hand spec is invalid: %v", err)
	}
	colourOf := func(name string) []byte {
		if len(name) == 0 {
			return nil
		}
		switch name[0] {
		case 'W', 'U', 'B', 'R', 'G':
			return []byte{name[0]}
		}
		return nil
	}
	// Precondition: green is genuinely scarce on the sheet, so an unbalanced
	// draw would miss it most of the time and the assertion below is live.
	var greenOnSheet int
	for _, sc := range sheet {
		if colourOf(sc.Name)[0] == 'G' {
			greenOnSheet++
		}
	}
	if greenOnSheet != 2 {
		t.Fatalf("precondition: sheet carries %d green commons, want 2", greenOnSheet)
	}
	const packs = 500
	greenPacks, whitePacks := 0, 0
	for i := 0; i < packs; i++ {
		ids := &booster.IDs{}
		pack, err := booster.Open(spec, rand.New(rand.NewPCG(uint64(i), 11)), ids, colourOf)
		if err != nil {
			t.Fatalf("Open pack %d: %v", i, err)
		}
		if len(pack) != 7 {
			t.Fatalf("pack %d has %d cards, want 7", i, len(pack))
		}
		names := map[string]bool{}
		for _, c := range pack {
			if names[c.Name] {
				t.Fatalf("pack %d repeats %q", i, c.Name)
			}
			names[c.Name] = true
			if names[c.ID] {
				t.Fatalf("pack %d reuses ID %q within itself", i, c.ID)
			}
			names[c.ID] = true
		}
		if ids.Count() != 7 {
			t.Errorf("pack %d allocated %d IDs, want 7 (balance must keep the drawn card's ID)", i, ids.Count())
		}
		mono := monoCount(colourOf, pack)
		if mono[4] > 0 {
			greenPacks++
		}
		if mono[0] > 0 {
			whitePacks++
		}
		for _, c := range pack {
			if c.Rarity != limited.Common {
				t.Errorf("pack %d carries %q at rarity %c off the common sheet", i, c.Name, c.Rarity)
			}
		}
	}
	if greenPacks != packs {
		t.Errorf("colour balance left green missing in %d of %d packs (white is plentiful, green is on the sheet: every pack must have one)", packs-greenPacks, packs)
	}
	if whitePacks != packs {
		t.Errorf("colour balance emptied white in %d of %d packs", packs-whitePacks, packs)
	}
}

// TestBoosterPodIsDeterministicAndSaltStable pins OpenPod: the same seed
// replays byte-identically, and growing the pod never shifts a pack another
// seat already holds.
func TestBoosterPodIsDeterministicAndSaltStable(t *testing.T) {
	sheet := fiveColourSheet()
	spec := setdata.Spec{
		Code: "TST", Name: "Test", PackSize: 7, MainMax: 40,
		Sheets: map[string][]setdata.SheetCard{
			"common":   sheet,
			"uncommon": {sheetCard("UncA", "1", limited.Uncommon)},
			"rare":     {sheetCard("RareA", "2", limited.Rare)},
			"mythic":   {sheetCard("MythA", "3", limited.Mythic)},
			"land":     {sheetCard("LandA", "9", limited.Land)},
		},
		Slots: []setdata.Slot{
			{Sheet: "common", Count: 4, BalanceColours: true},
			{RareMythic: &setdata.RareMythic{RareWeight: 2, MythicWeight: 1}, Count: 1},
			{Sheet: "uncommon", Count: 1},
			{Sheet: "land", Count: 1},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("precondition: hand spec is invalid: %v", err)
	}
	colourOf := func(name string) []byte {
		if len(name) > 0 && name[0] != 'L' {
			return []byte{name[0]}
		}
		return nil
	}
	openPod := func(seats, packs int) [][][]limited.CardRef {
		t.Helper()
		pod, err := booster.OpenPod(spec, 42, seats, packs, &booster.IDs{}, colourOf)
		if err != nil {
			t.Fatalf("OpenPod(%d seats, %d packs): %v", seats, packs, err)
		}
		return pod
	}
	if !reflect.DeepEqual(openPod(3, 2), openPod(3, 2)) {
		t.Error("the same seed opened two different pods")
	}
	// Precondition: the compared pods have the shapes the stability checks
	// index into.
	if got := openPod(3, 2); len(got) != 3 || len(got[0]) != 2 {
		t.Fatalf("pod shape = %dx%d, want 3x2", len(got), len(got[0]))
	}
	if !reflect.DeepEqual(openPod(3, 2)[1][0], openPod(2, 2)[1][0]) {
		t.Error("pack (seat 1, pack 0) changed when the seat count grew 2→3")
	}
	if !reflect.DeepEqual(openPod(3, 2)[0][1], openPod(3, 3)[0][1]) {
		t.Error("pack (seat 0, pack 1) changed when the pack count grew 2→3")
	}
	// One event-wide allocator: the pod's IDs are unique across every seat
	// and pack, in seat-major order.
	ids := &booster.IDs{}
	pod, err := booster.OpenPod(spec, 42, 3, 2, ids, colourOf)
	if err != nil {
		t.Fatalf("OpenPod: %v", err)
	}
	if ids.Count() != 3*2*7 {
		t.Errorf("allocator handed out %d IDs, want %d", ids.Count(), 3*2*7)
	}
	seen := map[string]bool{}
	for _, seat := range pod {
		for _, pack := range seat {
			for _, c := range pack {
				if seen[c.ID] {
					t.Errorf("ID %q appears twice across the pod", c.ID)
				}
				seen[c.ID] = true
			}
		}
	}
	if pod[0][0][0].ID != "c1" {
		t.Errorf("seat-major order broken: first pod card has ID %q, want c1", pod[0][0][0].ID)
	}
	if pod[2][1][6].ID != "c42" {
		t.Errorf("seat-major order broken: last pod card has ID %q, want c42", pod[2][1][6].ID)
	}
}

// TestBoosterRefusesMissingInputs checks the guard rails: no allocator, no
// seeded rng, or a balancing slot without a ColourOf all fail Open.
func TestBoosterRefusesMissingInputs(t *testing.T) {
	spec := setdata.Spec{
		Code: "TST", Name: "Test", PackSize: 1, MainMax: 10,
		Sheets: map[string][]setdata.SheetCard{"common": {sheetCard("C1", "1", limited.Common)}},
		Slots:  []setdata.Slot{{Sheet: "common", Count: 1, BalanceColours: true}},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("precondition: hand spec is invalid: %v", err)
	}
	rng := rand.New(rand.NewPCG(1, 1))
	if _, err := booster.Open(spec, rng, nil, nil); err == nil {
		t.Error("Open accepted a nil ID allocator")
	}
	if _, err := booster.Open(spec, nil, &booster.IDs{}, nil); err == nil {
		t.Error("Open accepted a nil rng")
	}
	if _, err := booster.Open(spec, rng, &booster.IDs{}, nil); err == nil {
		t.Error("Open accepted a balancing slot without a ColourOf")
	}
	if _, err := booster.OpenPod(spec, 1, 0, 1, &booster.IDs{}, nil); err == nil {
		t.Error("OpenPod accepted zero seats")
	}
}
