// Command pkgmap generates the public docs site's package map
// (docs/site/architecture/packages.md includes its output): one markdown
// table per tier listing every Go package in the module with its one-line
// purpose, its non-test and test line counts, its in-repo reverse importers
// and its status, followed by a mermaid tier-dependency summary built from
// the actual imports.
//
//	go run ./cmd/pkgmap > docs/site/architecture/_generated_packages.md
//	go run ./cmd/pkgmap -check    # exit 1 on an unclassified or stale entry
//
// Tier and status come from the committed classification file
// docs/site/architecture/packages.tiers.json (the single source for both);
// purpose is the first sentence of the package doc comment, falling back to
// the classification's optional "purpose" when the package has none. A
// package missing from the classification renders under "Unclassified".
//
// Packages are found by walking the source tree, not with `go list`, so
// tag-gated packages (lab/cmd/dzbroker, ...) are mapped too and
// their imports counted regardless of build constraints. Nested modules
// (web/, lab/training/experimental/gamepool/), testdata, and dot- or underscore-prefixed
// directories are skipped. Output is sorted and deterministic.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// classFile is the classification file, relative to the module root.
const classFile = "docs/site/architecture/packages.tiers.json"

// maxPurpose caps a purpose cell; a longer first sentence is cut with "…".
const maxPurpose = 180

func main() {
	root := flag.String("root", "", "module root (default: nearest go.mod above the working directory)")
	check := flag.Bool("check", false, "only verify the classification; exit 1 on an unclassified package or a stale entry")
	flag.Parse()
	r := *root
	if r == "" {
		var err error
		if r, err = findRoot(); err != nil {
			fmt.Fprintln(os.Stderr, "pkgmap:", err)
			os.Exit(2)
		}
	}
	m, err := load(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pkgmap:", err)
		os.Exit(2)
	}
	if *check {
		problems := m.problems()
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "pkgmap:", p)
		}
		if len(problems) > 0 {
			fmt.Fprintf(os.Stderr, "pkgmap: %d problem(s); edit %s\n", len(problems), classFile)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "pkgmap: %d packages, all classified\n", len(m.pkgs))
		return
	}
	w := bufio.NewWriter(os.Stdout)
	m.render(w)
	if err := w.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "pkgmap:", err)
		os.Exit(2)
	}
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}

// tier is one section of the map, in dependency order: a package in an
// earlier tier importing one in a later tier is a back-edge.
type tier struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Intro string `json:"intro,omitempty"`
}

type class struct {
	Tier    string `json:"tier"`
	Status  string `json:"status"`
	Purpose string `json:"purpose,omitempty"`
}

type classification struct {
	Tiers    []tier           `json:"tiers"`
	Packages map[string]class `json:"packages"`
}

// pkg is one directory's package, keyed by its module-relative path.
type pkg struct {
	path      string
	purpose   string
	loc, tloc int
	imports   []string // module-relative, non-test, sorted, deduplicated
	timports  []string // module-relative, test files only
	importers int      // non-test reverse importers
	timporter int      // test-only reverse importers
}

type pkgMap struct {
	module string
	cls    classification
	pkgs   []*pkg // sorted by path
	byPath map[string]*pkg
}

func load(root string) (*pkgMap, error) {
	module, err := modulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	m := &pkgMap{module: module, byPath: map[string]*pkg{}}
	raw, err := os.ReadFile(filepath.Join(root, classFile))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &m.cls); err != nil {
		return nil, fmt.Errorf("%s: %w", classFile, err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir // a nested module
			}
		}
		p, err := readPkg(root, path, module)
		if err != nil {
			return err
		}
		if p != nil {
			m.pkgs = append(m.pkgs, p)
			m.byPath[p.path] = p
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(m.pkgs, func(i, j int) bool { return m.pkgs[i].path < m.pkgs[j].path })
	for _, p := range m.pkgs {
		if c, ok := m.cls.Packages[p.path]; ok && p.purpose == "" {
			p.purpose = c.Purpose
		}
		for _, imp := range p.imports {
			if q := m.byPath[imp]; q != nil && q != p {
				q.importers++
			}
		}
	}
	for _, p := range m.pkgs {
		for _, imp := range p.timports {
			q := m.byPath[imp]
			if q == nil || q == p || contains(p.imports, imp) {
				continue
			}
			q.timporter++
		}
	}
	return m, nil
}

func modulePath(gomod string) (string, error) {
	raw, err := os.ReadFile(gomod)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
			return f[1], nil
		}
	}
	return "", fmt.Errorf("%s: no module line", gomod)
}

// readPkg parses one directory's Go files (imports and comments only),
// or returns nil when the directory holds none.
func readPkg(root, dir, module string) (*pkg, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return nil, err
	}
	p := &pkg{path: filepath.ToSlash(rel)}
	if p.path == "." {
		p.path = "(root)"
	}
	// doc.go first, then the rest in name order (ReadDir's): the first
	// comment that documents the package wins.
	for i, name := range files {
		if name == "doc.go" {
			copy(files[1:i+1], files[:i])
			files[0] = name
			break
		}
	}
	imps, timps := map[string]bool{}, map[string]bool{}
	fset := token.NewFileSet()
	for _, name := range files {
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		test := strings.HasSuffix(name, "_test.go")
		lines := bytes.Count(src, []byte("\n"))
		if len(src) > 0 && src[len(src)-1] != '\n' {
			lines++
		}
		if test {
			p.tloc += lines
		} else {
			p.loc += lines
		}
		f, err := parser.ParseFile(fset, name, src, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", p.path, name, err)
		}
		into := imps
		if test {
			into = timps
		}
		for _, s := range f.Imports {
			ip, err := strconv.Unquote(s.Path.Value)
			if err != nil {
				continue
			}
			if r, ok := strings.CutPrefix(ip, module+"/"); ok {
				into[r] = true
			}
		}
		if !test && p.purpose == "" {
			p.purpose = purposeOf(f, filepath.Base(dir))
		}
	}
	p.imports = sortedKeys(imps)
	p.timports = sortedKeys(timps)
	return p, nil
}

// purposeOf returns the first sentence of f's doc comment when that comment
// documents the package ("Package x ...", "Command x ...", or for a command
// "x is ..."), not just the file.
func purposeOf(f *ast.File, dirName string) string {
	if f.Doc == nil {
		return ""
	}
	text := f.Doc.Text()
	name := f.Name.Name
	ok := strings.HasPrefix(text, "Package "+name+" ") || strings.HasPrefix(text, "Package "+name+"'s ")
	if name == "main" {
		ok = strings.HasPrefix(text, "Command ") || strings.HasPrefix(text, dirName+" ")
	}
	if !ok {
		return ""
	}
	s := new(doc.Package).Synopsis(text)
	if r := []rune(s); len(r) > maxPurpose {
		s = strings.TrimSpace(string(r[:maxPurpose-1])) + "…"
	}
	return s
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(s []string, v string) bool {
	i := sort.SearchStrings(s, v)
	return i < len(s) && s[i] == v
}

// problems lists every unclassified package, every classified path that no
// longer exists, and every entry naming an undeclared tier.
func (m *pkgMap) problems() []string {
	var out []string
	tiers := map[string]bool{}
	for _, t := range m.cls.Tiers {
		tiers[t.ID] = true
	}
	for _, p := range m.pkgs {
		if _, ok := m.cls.Packages[p.path]; !ok {
			out = append(out, fmt.Sprintf("unclassified package %s", p.path))
		}
	}
	for _, path := range sortedKeys(classPaths(m.cls.Packages)) {
		c := m.cls.Packages[path]
		if m.byPath[path] == nil {
			out = append(out, fmt.Sprintf("classified path %s no longer exists", path))
		}
		if !tiers[c.Tier] {
			out = append(out, fmt.Sprintf("%s names undeclared tier %q", path, c.Tier))
		}
		if c.Status == "" {
			out = append(out, fmt.Sprintf("%s has no status", path))
		}
	}
	return out
}

func classPaths(m map[string]class) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

const unclassified = "unclassified"

// tierOf returns a package's tier id, or unclassified.
func (m *pkgMap) tierOf(path string) string {
	if c, ok := m.cls.Packages[path]; ok {
		for _, t := range m.cls.Tiers {
			if t.ID == c.Tier {
				return c.Tier
			}
		}
	}
	return unclassified
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.ReplaceAll(s, "\n", " ")
}

func (m *pkgMap) render(w io.Writer) {
	fmt.Fprintf(w, "<!-- Generated by `go run ./cmd/pkgmap`; do not edit. Classification: %s -->\n\n", classFile)
	tiers := append([]tier(nil), m.cls.Tiers...)
	tiers = append(tiers, tier{ID: unclassified, Title: "Unclassified", Intro: "Packages missing from `" + classFile + "`; `go run ./cmd/pkgmap -check` fails until each is given a tier and status."})
	fmt.Fprintf(w, "*%d packages.*\n\n", len(m.pkgs))
	for _, t := range tiers {
		var rows []*pkg
		for _, p := range m.pkgs {
			if m.tierOf(p.path) == t.ID {
				rows = append(rows, p)
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(w, "## %s\n\n", t.Title)
		if t.Intro != "" {
			fmt.Fprintf(w, "%s\n\n", t.Intro)
		}
		fmt.Fprintln(w, "| Package | Purpose | LOC | Test LOC | Imp | Status |")
		fmt.Fprintln(w, "|---|---|--:|--:|--:|---|")
		for _, p := range rows {
			purpose := p.purpose
			if purpose == "" {
				purpose = "*(no package doc)*"
			}
			status := m.cls.Packages[p.path].Status
			if status == "" {
				status = "?"
			}
			imp := strconv.Itoa(p.importers)
			if p.timporter > 0 {
				imp += fmt.Sprintf(" +%d", p.timporter)
			}
			fmt.Fprintf(w, "| `%s` | %s | %d | %d | %s | %s |\n", p.path, cell(purpose), p.loc, p.tloc, imp, cell(status))
		}
		fmt.Fprintln(w)
	}
	m.renderTierGraph(w, tiers)
}

type edgeKey struct{ from, to int }

// renderTierGraph writes the mermaid tier→tier summary: each edge counts the
// distinct (importer, imported) non-test package pairs between two tiers, and
// an edge from an earlier tier into a later one is a back-edge.
func (m *pkgMap) renderTierGraph(w io.Writer, tiers []tier) {
	index := map[string]int{}
	for i, t := range tiers {
		index[t.ID] = i
	}
	counts := map[edgeKey]int{}
	sizes := make([]int, len(tiers))
	type pair struct{ from, to string }
	var back []pair
	for _, p := range m.pkgs {
		fi := index[m.tierOf(p.path)]
		sizes[fi]++
		for _, imp := range p.imports {
			if m.byPath[imp] == nil {
				continue
			}
			ti := index[m.tierOf(imp)]
			if fi == ti {
				continue
			}
			counts[edgeKey{fi, ti}]++
			if fi < ti && tiers[fi].ID != unclassified && tiers[ti].ID != unclassified {
				back = append(back, pair{p.path, imp})
			}
		}
	}
	keys := make([]edgeKey, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].from != keys[j].from {
			return keys[i].from < keys[j].from
		}
		return keys[i].to < keys[j].to
	})
	fmt.Fprintln(w, "## Tier dependencies")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Generated from the non-test imports (build tags ignored). An arrow points from the importing tier to the imported one and counts distinct package pairs; **red arrows are back-edges**, a tier importing one listed after it.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```mermaid")
	fmt.Fprintln(w, "graph TD")
	for i, t := range tiers {
		if sizes[i] == 0 {
			continue
		}
		fmt.Fprintf(w, "  t%d[\"%s<br/>%d packages\"]\n", i, strings.NewReplacer(`"`, "'", "`", "").Replace(t.Title), sizes[i])
	}
	var backLinks []int
	for n, k := range keys {
		label := strconv.Itoa(counts[k])
		if k.from < k.to {
			label += " back"
			backLinks = append(backLinks, n)
		}
		fmt.Fprintf(w, "  t%d -->|%s| t%d\n", k.from, label, k.to)
	}
	for _, n := range backLinks {
		fmt.Fprintf(w, "  linkStyle %d stroke:#d32f2f,stroke-width:2px\n", n)
	}
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
	if len(back) == 0 {
		fmt.Fprintln(w, "No back-edges.")
		return
	}
	fmt.Fprintf(w, "??? note \"%d back-edge import(s)\"\n\n", len(back))
	fmt.Fprintln(w, "    | Importer | Imports | Tiers |")
	fmt.Fprintln(w, "    |---|---|---|")
	for _, b := range back {
		fmt.Fprintf(w, "    | `%s` | `%s` | %s → %s |\n", b.from, b.to, m.tierOf(b.from), m.tierOf(b.to))
	}
	fmt.Fprintln(w)
}
