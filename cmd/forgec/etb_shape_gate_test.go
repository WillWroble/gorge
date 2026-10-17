package main

import (
	"testing"

	"github.com/adams-shaun/gorge/engine/effects"
	"github.com/adams-shaun/gorge/testkit/testutil"
)

// TestETBCopyShapeGateCountsLoudNoteCarriersUnsupported pins the coverage
// report's shape gate: a card whose ETBReplacement:Copy DB$ Clone body the
// engine declines (Cursed Mirror's Duration$) is primitive-complete but
// takes the loud unimplemented-API Note when cast, so the gate must count it
// unsupported; Phantasmal Image (AddTypes$/AddTriggers$/AddSVars$ riders,
// now offered) must stay playable.
func TestETBCopyShapeGateCountsLoudNoteCarriersUnsupported(t *testing.T) {
	r := testutil.CorpusRegistry(t)
	sup := effects.Supported()
	mirror, ok1 := r.Lookup("Cursed Mirror")
	image, ok2 := r.Lookup("Phantasmal Image")
	if !ok1 || !ok2 {
		t.Fatal("corpus missing Cursed Mirror / Phantasmal Image")
	}
	// Precondition: both are primitive-complete, so only the gate can differ.
	if m := r.Unsupported(mirror, sup); len(m) != 0 {
		t.Fatalf("Cursed Mirror unexpectedly unsupported by primitive: %v", m)
	}
	if len(etbCopyShapeGate(mirror)) == 0 {
		t.Fatal("Cursed Mirror's declined ETB Clone body is not gated")
	}
	if g := etbCopyShapeGate(image); len(g) != 0 {
		t.Fatalf("Phantasmal Image gated: %v", g)
	}
	r.ShapeGate = etbCopyShapeGate
	defer func() { r.ShapeGate = nil }()
	if len(r.Unsupported(mirror, sup)) == 0 {
		t.Error("Cursed Mirror counted playable with the gate set")
	}
	if m := r.Unsupported(image, sup); len(m) != 0 {
		t.Errorf("Phantasmal Image counted unsupported: %v", m)
	}
}
