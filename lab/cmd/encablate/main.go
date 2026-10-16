// Command encablate is a RESEARCH tool (2026-10-10 encoding ablation): it
// turns searchteacher label corpora into a family-tagged sparse token dump
// for the Python ablation harness (scripts in
// archived 2026-10-10-encoding-ablation.md (docs/design/ARCHIVE.md)).
//
// Every record's redacted view is walked by the mzenc port with
// Encoder.Hook set, so each emitted feature is attributed to the walker
// FAMILY that originated it (before passToParent propagation). Card-instance
// namespaces are abstracted out of the token (the per-zone aggregate copies
// the walker already propagates are kept), and card identity is emitted
// separately as zone|name tokens so the harness can swap names for an IR bag.
//
// Output (gzip JSONL, one object per label record) plus <out>.vocab.json
// (token strings, family names) and, with -ir, <out>.ir.json: name -> IR-bag
// tokens for every card name seen.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/adams-shaun/gorge/engine/cards"
	"github.com/adams-shaun/gorge/engine/decision"
	"github.com/adams-shaun/gorge/engine/state"
	"github.com/adams-shaun/gorge/engine/view"
	"github.com/adams-shaun/gorge/lab/training/mzenc"
)

type cand struct {
	Choices []int   `json:"choices"`
	Bot     bool    `json:"bot"`
	Value   float64 `json:"value"`
}

type extras struct {
	DiagOppHand []view.CardView `json:"diag_opp_hand"`
}

type rec struct {
	Pair          string            `json:"pair"`
	GameIndex     int               `json:"game_index"`
	Seed          uint64            `json:"seed"`
	Seat          state.PlayerID    `json:"seat"`
	Kind          decision.Kind     `json:"kind"`
	Turn          int32             `json:"turn"`
	View          view.View         `json:"view"`
	Options       []decision.Option `json:"options"`
	Candidates    []cand            `json:"candidates"`
	TeacherChoice int               `json:"teacher_choice"`
	BotIndex      int               `json:"bot_index"`
	Outcome       float64           `json:"outcome"`
	OutcomeKnown  bool              `json:"outcome_known"`
	Extras        *extras           `json:"extras"`
}

type out struct {
	Src     string    `json:"src"`
	Game    string    `json:"game"`
	Deck    string    `json:"deck"`
	ODeck   string    `json:"odeck"`
	Kind    string    `json:"kind"`
	Turn    int32     `json:"turn"`
	Outcome float64   `json:"y"`
	OK      bool      `json:"ok"`
	Teacher int       `json:"t"`
	Bot     int       `json:"b"`
	Fam     []int     `json:"fam"` // parallel to Tok
	Tok     []int     `json:"tok"`
	Names   []int     `json:"names"` // zone|name identity tokens (vocab ids)
	Cands   [][]int   `json:"cands"` // per candidate option tokens (vocab ids)
	CNames  [][]int   `json:"cnames"`
	OppHand []string  `json:"opphand,omitempty"`
	HasDiag bool      `json:"diag"`
	CVals   []float64 `json:"cv"`
}

var (
	vocab   = map[string]int{}
	vocabL  []string
	fams    = map[string]int{}
	famL    []string
	seenCrd = map[string]bool{}
)

func id(m map[string]int, l *[]string, s string) int {
	if v, ok := m[s]; ok {
		return v
	}
	m[s] = len(*l)
	*l = append(*l, s)
	return m[s]
}

func tok(s string) int { return id(vocab, &vocabL, s) }
func fam(s string) int { return id(fams, &famL, s) }

// zoneNames are the walker namespaces whose children are card instances.
var zoneNames = map[string]bool{"Battlefield": true, "Graveyard": true, "Hand": true, "ExileZone": true, "Stack": true, "CommandZone": true, "Commander": true}

func base(n string) string {
	if i := strings.LastIndexByte(n, '#'); i >= 0 {
		return n[:i]
	}
	return n
}

// path returns the namespace chain below root, with each card-instance
// component replaced by "*" and a flag saying whether one was replaced.
func path(n *mzenc.Node) ([]string, bool) {
	var p []string
	for x := n; x != nil && x.Parent() != nil; x = x.Parent() {
		p = append(p, base(x.Name()))
	}
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
	card := false
	for i := 1; i < len(p); i++ {
		if zoneNames[p[i-1]] && !card {
			p[i] = "*"
			card = true
		}
	}
	return p, card
}

func side(p []string) string {
	if len(p) == 0 {
		return ""
	}
	switch p[0] {
	case "Player":
		return "Me."
	case "Opponent":
		return "Opp."
	}
	return ""
}

// family classifies an origin node + key into an ablation family.
func family(origin *mzenc.Node, key string) string {
	p, card := path(origin)
	k := base(key)
	if i := strings.IndexByte(k, '@'); i >= 0 {
		k = k[:i]
	}
	if len(p) == 0 {
		switch {
		case k == "Stack" || k == "Exile":
			return k
		case strings.ToUpper(k) == k && !strings.Contains(k, " "):
			return "G.StepOrDecisionType"
		}
		return "G.DecisionsText"
	}
	s := side(p)
	if s == "" {
		return p[0] // Stack / Exile subtree
	}
	if len(p) == 1 {
		switch k {
		case "Battlefield", "Graveyard", "Hand", "ManaPool", "CommandZone":
			return s + "ZoneMarkers"
		case "LifeTotal", "LibraryCount", "CanPlayLand", "IsActivePlayer", "IsDecisionPlayer":
			return s + k
		}
		return s + "PlayerMisc"
	}
	z := p[1]
	if z == "ManaPool" {
		return s + "ManaPool"
	}
	if !card && len(p) == 2 {
		return s + z + ".name"
	}
	return s + z + ".props"
}

func main() {
	outPath := flag.String("out", "", "output .jsonl.gz")
	cardsDir := flag.String("cards", "", "with a corpus dir: also write <out>.ir.json")
	flag.Parse()
	of, err := os.Create(*outPath)
	must(err)
	gz := gzip.NewWriter(of)
	enc := json.NewEncoder(gz)
	for _, in := range flag.Args() {
		n := 0
		each(in, func(r *rec) {
			o := encode(r, in)
			must(enc.Encode(o))
			n++
		})
		fmt.Fprintf(os.Stderr, "%s: %d records\n", in, n)
	}
	must(gz.Close())
	must(of.Close())
	vf, err := os.Create(*outPath + ".vocab.json")
	must(err)
	must(json.NewEncoder(vf).Encode(map[string]any{"tokens": vocabL, "families": famL}))
	must(vf.Close())
	if *cardsDir != "" {
		reg, err := cards.OpenCorpus(*cardsDir)
		must(err)
		names := make([]string, 0, len(seenCrd))
		for n := range seenCrd {
			names = append(names, n)
		}
		sort.Strings(names)
		ir := map[string][]string{}
		miss := 0
		for _, n := range names {
			c, ok := reg.Lookup(n)
			if !ok || len(c.Faces) == 0 {
				miss++
				continue
			}
			ir[n] = irBag(c.Faces[0])
		}
		fmt.Fprintf(os.Stderr, "ir: %d cards, %d missing\n", len(ir), miss)
		jf, err := os.Create(*outPath + ".ir.json")
		must(err)
		must(json.NewEncoder(jf).Encode(ir))
		must(jf.Close())
	}
}

func irBag(f *cards.Face) []string {
	var t []string
	for _, ty := range f.Types {
		t = append(t, "type:"+ty)
	}
	t = append(t, "cmc:"+strconv.Itoa(int(f.Cmc())))
	for _, c := range "WUBRG" {
		if strings.ContainsRune(f.ManaCost, c) {
			t = append(t, "col:"+string(c))
		}
	}
	if f.PT != "" {
		t = append(t, "pow:"+strconv.Itoa(int(f.Power())), "tou:"+strconv.Itoa(int(f.Toughness())))
	}
	for _, k := range f.Keywords {
		if i := strings.IndexByte(k, ':'); i >= 0 {
			k = k[:i]
		}
		t = append(t, "kw:"+k)
	}
	var sa func(pre string, a *cards.SA, d int)
	sa = func(pre string, a *cards.SA, d int) {
		if a == nil || d > 4 {
			return
		}
		t = append(t, pre+a.Kind+":"+a.API, "api:"+a.API)
		sa(pre, a.Sub, d+1)
	}
	for _, a := range f.Abilities {
		sa("sa:", a, 0)
	}
	for _, tr := range f.Triggers {
		t = append(t, "trig:"+tr.Mode)
		sa("trigfx:", tr.Effect, 0)
	}
	for _, st := range f.Statics {
		t = append(t, "static:"+st.Mode)
	}
	for _, rp := range f.Repls {
		t = append(t, "repl:"+rp.Event)
		sa("replfx:", rp.With, 0)
	}
	return t
}

func encode(r *rec, src string) out {
	pp := strings.Split(r.Pair, ":")
	o := out{Src: src, Game: fmt.Sprintf("%s#%d#%d", r.Pair, r.Seed, r.GameIndex), Kind: string(r.Kind), Turn: r.Turn,
		Outcome: r.Outcome, OK: r.OutcomeKnown, Teacher: r.TeacherChoice, Bot: r.BotIndex}
	if len(pp) == 2 && int(r.Seat) < 2 {
		o.Deck, o.ODeck = pp[r.Seat], pp[1-r.Seat]
	}
	dt := 0
	if r.Kind == "target" {
		dt = 3
	}
	seen := map[[2]int]bool{}
	mzenc.ProcessStateHook(r.View, r.Seat, dt, string(r.Kind), func(origin, at *mzenc.Node, key string) {
		ap, card := path(at)
		if card {
			return // per-card-instance copy: identity carried by Names
		}
		f := fam(family(origin, key))
		t := tok(strings.Join(ap, "/") + "|" + key)
		if !seen[[2]int{f, t}] {
			seen[[2]int{f, t}] = true
			o.Fam = append(o.Fam, f)
			o.Tok = append(o.Tok, t)
		}
	})
	// extra families the mzenc walker does not emit (the gaps): counts.
	add := func(f, s string) {
		o.Fam = append(o.Fam, fam(f))
		o.Tok = append(o.Tok, tok(s))
	}
	names := map[state.ObjID]string{}
	for _, p := range r.View.Players {
		sd := "Opp"
		if p.ID == r.Seat {
			sd = "Me"
		}
		for _, n := range thermo(p.HandSize) {
			add("X."+sd+".HandSize", sd+"|handsize@"+n)
		}
		for _, n := range thermo(p.GraveyardSize) {
			add("X."+sd+".ZoneCounts", sd+"|gysize@"+n)
		}
		for _, n := range thermo(len(p.Exile)) {
			add("X."+sd+".ZoneCounts", sd+"|exilesize@"+n)
		}
		zones := []struct {
			z  string
			cs []view.CardView
		}{{"bf", p.Battlefield}, {"hand", p.Hand}, {"gy", p.Graveyard}, {"exile", p.Exile}}
		for _, z := range zones {
			for _, c := range z.cs {
				names[c.ID] = c.Name
				seenCrd[c.Name] = true
				o.Names = append(o.Names, tok("N|"+sd+"|"+z.z+"|"+c.Name))
			}
		}
	}
	for _, s := range r.View.Stack {
		names[s.ID] = s.Name
		seenCrd[s.Name] = true
		o.Names = append(o.Names, tok("N|stack|"+s.Name))
	}
	for _, c := range r.Candidates {
		var ct, cn []int
		if len(c.Choices) == 0 {
			ct = append(ct, tok("O|none"))
		}
		ct = append(ct, tok("O|n@"+strconv.Itoa(min(len(c.Choices), 5))))
		for _, i := range c.Choices {
			if i < 0 || i >= len(r.Options) {
				continue
			}
			op := r.Options[i]
			ct = append(ct, tok("O|kind|"+string(op.Kind)))
			if nm, ok := names[op.Obj]; ok {
				cn = append(cn, tok("ON|"+string(op.Kind)+"|"+nm))
				seenCrd[nm] = true
			} else {
				cn = append(cn, tok("ON|"+string(op.Kind)+"|?"))
			}
		}
		if c.Bot {
			ct = append(ct, tok("O|bot"))
		}
		o.Cands = append(o.Cands, ct)
		o.CNames = append(o.CNames, cn)
		o.CVals = append(o.CVals, c.Value)
	}
	if r.Extras != nil {
		o.HasDiag = true
		o.OppHand = []string{}
		for _, c := range r.Extras.DiagOppHand {
			o.OppHand = append(o.OppHand, c.Name)
			seenCrd[c.Name] = true
		}
	}
	return o
}

func thermo(n int) []string {
	var s []string
	for _, b := range []int{1, 2, 3, 4, 5, 7, 10, 15} {
		if n >= b {
			s = append(s, strconv.Itoa(b))
		}
	}
	return s
}

func each(path string, f func(*rec)) {
	fh, err := os.Open(path)
	must(err)
	defer fh.Close()
	var rd io.Reader = fh
	if strings.HasSuffix(path, ".gz") {
		g, err := gzip.NewReader(fh)
		must(err)
		rd = g
	}
	br := bufio.NewReaderSize(rd, 1<<20)
	dec := json.NewDecoder(br)
	for {
		var r rec
		if err := dec.Decode(&r); err == io.EOF {
			return
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "%s: decode: %v\n", path, err)
			return
		}
		f(&r)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
