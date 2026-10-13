// Command gen regenerates the embedded per-set booster specs in
// limited/setdata/sets from the compliance manifest plus the hand-authored
// header. It is a build-time tool only: the setdata package itself stays
// pure and reads nothing but its embedded files.
//
//	go run ./internal/gen -sets FDN,BLB
//
// (wired as the //go:generate directive in the parent package; run from
// limited/setdata, where headers/, sets/ and ../../compliance/manifests
// resolve). The output is deterministic: cards sorted by collector number
// then name, each sheet card carrying the lowest main-set collector number.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/adams-shaun/gorge/limited"
	"github.com/adams-shaun/gorge/limited/setdata"
)

// header is the hand-authored half of a set spec: the product facts no
// manifest carries (the pack shape) plus the land-sheet names.
type header struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	MainMax   int    `json:"main_max"`
	PackSize  int    `json:"pack_size"`
	LandSheet struct {
		Names []string `json:"names"`
	} `json:"land_sheet"`
	Slots []setdata.Slot `json:"slots"`
}

// manifestCard mirrors one entry of compliance/manifests/<SET>.json.
type manifestCard struct {
	Name    string   `json:"name"`
	Numbers []string `json:"numbers"`
	Rarity  string   `json:"rarity"`
}

type manifest struct {
	Code  string         `json:"code"`
	Cards []manifestCard `json:"cards"`
}

// rarityLetters maps the manifest's rarity words to limited.Rarity.
var rarityLetters = map[string]limited.Rarity{
	"COMMON":   limited.Common,
	"UNCOMMON": limited.Uncommon,
	"RARE":     limited.Rare,
	"MYTHIC":   limited.Mythic,
	"LAND":     limited.Land,
	"SPECIAL":  limited.Special,
}

// sheetNames are the fixed sheet keys of the spec shape.
var sheetNames = []string{"common", "uncommon", "rare", "mythic", "land"}

// Generate builds one set's spec JSON from its header and manifest bytes.
// A card joins a C/U/R/M sheet when at least one numeric collector number
// is <= main_max, bucketed by manifest rarity, carrying the lowest such
// number; cards named in the header's land sheet join the land sheet
// instead (and are removed from the other sheets), carrying the lowest
// main-set number or, when the printing sits outside the main runs (BLB's
// basics), the lowest number it has. Numbers that are not numeric are
// skipped. Cards outside the main runs are not pack material.
func Generate(headerJSON, manifestJSON []byte) ([]byte, error) {
	var h header
	if err := strictUnmarshal(headerJSON, &h); err != nil {
		return nil, fmt.Errorf("gen: header: %w", err)
	}
	var m manifest
	// The manifest carries many set-level fields this tool does not model,
	// so only the header is decoded strictly.
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return nil, fmt.Errorf("gen: manifest: %w", err)
	}
	if h.Code == "" || h.Name == "" || h.MainMax <= 0 || h.PackSize <= 0 {
		return nil, fmt.Errorf("gen: header needs code, name, positive main_max and pack_size")
	}
	if m.Code != h.Code {
		return nil, fmt.Errorf("gen: header is %s but manifest is %s", h.Code, m.Code)
	}
	if len(h.Slots) == 0 || len(h.LandSheet.Names) == 0 {
		return nil, fmt.Errorf("gen: header needs slots and land_sheet names")
	}
	land := map[string]bool{}
	for _, n := range h.LandSheet.Names {
		land[n] = true
	}

	buckets := map[string][]setdata.SheetCard{}
	for _, c := range m.Cards {
		nums := numericNumbers(c.Numbers)
		if len(nums) == 0 {
			continue // non-numeric-only printing: not pack material
		}
		lowest := nums[0]
		mainLowest := 0
		for _, n := range nums {
			if n <= h.MainMax && (mainLowest == 0 || n < mainLowest) {
				mainLowest = n
			}
		}
		r, ok := rarityLetters[c.Rarity]
		if !ok {
			return nil, fmt.Errorf("gen: %s: unknown rarity %q", c.Name, c.Rarity)
		}
		var sheet string
		number := lowest
		switch {
		case land[c.Name]:
			sheet, r = "land", limited.Land
			if mainLowest != 0 {
				number = mainLowest
			}
		case mainLowest != 0:
			switch r {
			case limited.Common, limited.Uncommon, limited.Rare, limited.Mythic:
				sheet = sheetNameFor(r)
			default:
				return nil, fmt.Errorf("gen: %s: %s-rarity card %d inside the main run is neither a land-sheet name nor a play rarity",
					h.Code, c.Rarity, mainLowest)
			}
		default:
			continue // bonus-sheet-only printing
		}
		buckets[sheet] = append(buckets[sheet], setdata.SheetCard{
			Name:   c.Name,
			Number: strconv.Itoa(number),
			Rarity: r,
		})
	}

	sheets := map[string][]setdata.SheetCard{}
	for _, name := range sheetNames {
		cards := buckets[name]
		if len(cards) == 0 {
			return nil, fmt.Errorf("gen: %s: sheet %q came out empty", h.Code, name)
		}
		sort.Slice(cards, func(i, j int) bool { return lessSheet(cards[i], cards[j]) })
		sheets[name] = cards
	}

	spec := setdata.Spec{
		Code:     h.Code,
		Name:     h.Name,
		PackSize: h.PackSize,
		MainMax:  h.MainMax,
		Sheets:   sheets,
		Slots:    h.Slots,
		Source:   fmt.Sprintf("compliance/manifests/%s.json (XMage-derived, MIT) + gorge header", h.Code),
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("gen: %s: generated spec is invalid: %w", h.Code, err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(spec); err != nil {
		return nil, fmt.Errorf("gen: %s: %w", h.Code, err)
	}
	return buf.Bytes(), nil
}

// sheetNameFor buckets a play rarity into its sheet key.
func sheetNameFor(r limited.Rarity) string {
	switch r {
	case limited.Common:
		return "common"
	case limited.Uncommon:
		return "uncommon"
	case limited.Rare:
		return "rare"
	default:
		return "mythic"
	}
}

// numericNumbers keeps the collector numbers that parse as integers, in
// manifest order (deduplicated).
func numericNumbers(in []string) []int {
	seen := map[int]bool{}
	var out []int
	for _, s := range in {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// lessSheet orders a sheet by numeric collector number, then name.
func lessSheet(a, b setdata.SheetCard) bool {
	an, aerr := strconv.Atoi(a.Number)
	bn, berr := strconv.Atoi(b.Number)
	if aerr == nil && berr == nil {
		if an != bn {
			return an < bn
		}
		return a.Name < b.Name
	}
	if a.Number != b.Number {
		return a.Number < b.Number
	}
	return a.Name < b.Name
}

// strictUnmarshal decodes v refusing fields the types do not declare.
func strictUnmarshal(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func main() {
	sets := flag.String("sets", "", "comma-separated set codes to (re)generate")
	root := flag.String("root", ".", "limited/setdata directory holding headers/ and sets/")
	manifests := flag.String("manifests", "../../compliance/manifests", "directory holding compliance/manifests/<SET>.json")
	flag.Parse()
	if *sets == "" {
		fmt.Fprintln(os.Stderr, "usage: gen -sets FDN,BLB [-root .] [-manifests ../../compliance/manifests]")
		os.Exit(2)
	}
	for _, code := range strings.Split(*sets, ",") {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if err := run(code, *root, *manifests); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", filepath.Join(*root, "sets", code+".json"))
	}
}

// run regenerates one set's embedded file from its header and manifest.
func run(code, root, manifests string) error {
	headerJSON, err := os.ReadFile(filepath.Join(root, "headers", code+".json"))
	if err != nil {
		return err
	}
	manifestJSON, err := os.ReadFile(filepath.Join(manifests, code+".json"))
	if err != nil {
		return err
	}
	out, err := Generate(headerJSON, manifestJSON)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "sets", code+".json"), out, 0o644)
}
