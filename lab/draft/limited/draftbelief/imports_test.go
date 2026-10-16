package draftbelief_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// forbiddenImports are the imports the draft-spec invariant bans from the
// pure lab/draft/limited/ packages: no clock, no file or network I/O, no legacy
// math/rand, and none of the engine tiers. Draft-RL spec §2.1 adds the
// event layer and draftlab: features see a PickView and the SetTable, never
// the pod. allowedGorge is the complete list of gorge imports.
var forbiddenImports = []string{
	"time",
	"os",
	"io/fs",
	"net",
	"net/http",
	"math/rand",
	"github.com/adams-shaun/gorge/engine/rules",
	"github.com/adams-shaun/gorge/engine/state",
	"github.com/adams-shaun/gorge/engine/events",
	"github.com/adams-shaun/gorge/engine/effects",
	"github.com/adams-shaun/gorge/server/host",
	"github.com/adams-shaun/gorge/engine/view",
	"github.com/adams-shaun/gorge/engine/seat",
	"github.com/adams-shaun/gorge/bots",
	"github.com/adams-shaun/gorge/bots/botpolicy",
	"github.com/adams-shaun/gorge/lab/draft/limited/booster",
	"github.com/adams-shaun/gorge/lab/draft/limited/setdata",
	"github.com/adams-shaun/gorge/internal",
	"github.com/adams-shaun/gorge/lab/draft/event",
	"github.com/adams-shaun/gorge/lab/draft/eventhost",
	"github.com/adams-shaun/gorge/lab/draft/draftlab",
	"github.com/adams-shaun/gorge/lab/draft/limited/pickbot",
}

var allowedGorge = map[string]bool{
	"github.com/adams-shaun/gorge/lab/draft/limited":           true,
	"github.com/adams-shaun/gorge/lab/draft/limited/draft":     true,
	"github.com/adams-shaun/gorge/lab/draft/limited/draftfeat": true,
}

func TestImportsStayPure(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	if len(files) == 0 {
		t.Fatal("no non-test Go files found: the purity scan scanned nothing")
	}
	sort.Strings(files)
	for _, path := range files {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, "github.com/adams-shaun/gorge") && !allowedGorge[p] {
				t.Errorf("%s imports %q: draftbelief may import only limited, lab/draft/limited/draft and lab/draft/limited/draftfeat: it reads one seat's observation log and public set data, never the pod or the event", path, p)
			}
			for _, bad := range forbiddenImports {
				// math/rand is matched exactly so its v2 successor — the
				// only rand the spec allows — is never caught by a prefix.
				if p == bad || (bad != "math/rand" && strings.HasPrefix(p, bad+"/")) {
					t.Errorf("%s imports %q: %s must stay pure (spec invariant 3)", f.Name.Name, p, dir)
				}
			}
		}
	}
}
