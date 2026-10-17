package gorged

import "testing"

// TestServeFlagSetIsTheRealFlagSet pins the exported seam: a fresh copy of
// the flags Main parses, that reports an unknown flag as an error instead of
// exiting, and reads back parsed values by name.
func TestServeFlagSetIsTheRealFlagSet(t *testing.T) {
	fs := ServeFlagSet()
	if err := fs.Parse([]string{"-tables", "7", "-prewarm-art-only"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := fs.Lookup("tables").Value.String(); got != "7" {
		t.Errorf("-tables = %q, want 7", got)
	}
	if got := fs.Lookup("prewarm-art-only").Value.String(); got != "true" {
		t.Errorf("-prewarm-art-only = %q, want true", got)
	}
	if err := ServeFlagSet().Parse([]string{"-no-such-flag"}); err == nil {
		t.Error("an unknown flag parsed without error")
	}
	_, def := serveFlags()
	if got := ServeFlagSet().Lookup("tables").Value.String(); got != "4" || def.tables != 4 {
		t.Errorf("fresh -tables default = %q (serveFlags %d), want 4: the copies are not independent", got, def.tables)
	}
}
