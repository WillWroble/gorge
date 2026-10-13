package effects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoCtxRememberedAppendDedupesAgainstTheCapture is the ratchet for the
// Coveted Falcon class: a Remember* rider that guards its Ctx.Remembered
// append with a plain `!targetIn(c.Remembered, ..)` skips the write when the
// trigger's capture already names the object, and Remembered$Amount then reads
// zero. Every such append goes through rememberCtxOnce.
func TestNoCtxRememberedAppendDedupesAgainstTheCapture(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob: %v (%d files)", err, len(files))
	}
	scanned := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for i, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "!targetIn(c.Remembered,") {
				t.Errorf("%s:%d guards a Remembered append with a plain membership test; use rememberCtxOnce", f, i+1)
			}
		}
	}
	if scanned < 50 {
		t.Fatalf("census scanned only %d files", scanned)
	}
}
