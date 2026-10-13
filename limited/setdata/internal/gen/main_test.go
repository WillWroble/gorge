package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateReproducesCommittedSets regenerates every set that has a
// hand-authored header, in memory, and asserts the committed file under
// sets/ is byte-identical — so a manifest change (a new printing, a rarity
// correction at the XMage pin) shows up as a diff instead of silently
// drifting from the committed spec.
func TestGenerateReproducesCommittedSets(t *testing.T) {
	// The generator runs from limited/setdata; the test runs from this
	// package's directory three levels deeper.
	root := "../.."
	manifests := "../../../../compliance/manifests"

	headerEntries, err := os.ReadDir(filepath.Join(root, "headers"))
	if err != nil {
		t.Fatalf("read headers: %v", err)
	}
	var codes []string
	for _, e := range headerEntries {
		if name := e.Name(); strings.HasSuffix(name, ".json") {
			codes = append(codes, strings.TrimSuffix(name, ".json"))
		}
	}
	if len(codes) != 2 {
		t.Fatalf("expected the two committed headers (FDN, BLB), found %d: %v", len(codes), codes)
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			headerJSON, err := os.ReadFile(filepath.Join(root, "headers", code+".json"))
			if err != nil {
				t.Fatalf("read header: %v", err)
			}
			manifestJSON, err := os.ReadFile(filepath.Join(manifests, code+".json"))
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			got, err := Generate(headerJSON, manifestJSON)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			if len(got) < 1000 {
				t.Fatalf("generated spec for %s is only %d bytes: the generator produced nothing", code, len(got))
			}
			want, err := os.ReadFile(filepath.Join(root, "sets", code+".json"))
			if err != nil {
				t.Fatalf("read committed set file: %v", err)
			}
			if !bytes.Equal(got, want) {
				// Show the first diverging line so the drift is readable.
				gotLines := strings.Split(string(got), "\n")
				wantLines := strings.Split(string(want), "\n")
				for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
					g, w := "", ""
					if i < len(gotLines) {
						g = gotLines[i]
					}
					if i < len(wantLines) {
						w = wantLines[i]
					}
					if g != w {
						t.Fatalf("sets/%s.json is stale: regenerate with `go generate ./` in limited/setdata.\nfirst differing line %d:\n  generated: %s\n  committed: %s", code, i+1, g, w)
					}
				}
				t.Fatalf("sets/%s.json differs from the regenerated spec", code)
			}
		})
	}
}

// TestGenerateRejectsMismatchedManifest proves the generator fails loudly
// when the header and manifest disagree about the set.
func TestGenerateRejectsMismatchedManifest(t *testing.T) {
	h := []byte(`{"code":"FDN","name":"Foundations","main_max":271,"pack_size":14,
		"land_sheet":{"names":["Plains"]},
		"slots":[{"sheet":"land","count":1}]}`)
	m := []byte(`{"code":"BLB","cards":[]}`)
	if _, err := Generate(h, m); err == nil {
		t.Fatal("Generate accepted a header and manifest for different sets")
	}
}
